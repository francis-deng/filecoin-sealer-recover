package sealer

import (
	"bytes"
	"context"
	"fmt"
	"github.com/filecoin-project/go-commp-utils/writer"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/specs-storage/storage"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"io"
	"io/fs"
	"io/ioutil"
	"math/bits"
	"os"
)

var log = logging.Logger("ssb")

type SectorSealer struct {
	sp ffiwrapper.SectorProvider

	sb *ffiwrapper.Sealer

	ref *storage.SectorRef

	dataSizes []abi.UnpaddedPieceSize

	sectorSize abi.SectorSize
	pieces []abi.PieceInfo

	ticket abi.SealRandomness
	p1 storage.PreCommit1Out

	cids storage.SectorCids

	seed abi.InteractiveSealRandomness
	c1 storage.Commit1Out

	proof storage.Proof
}

func New(root string) *SectorSealer {
	rootFullPath, err := homedir.Expand(root)
	if err != nil {
		panic(err)
	}
	if err=os.Mkdir(rootFullPath, fs.FileMode(755));err != nil && !os.IsExist(err) {
		panic(err)
	}

	sp := &basicfs.Provider{
		Root: rootFullPath,
	}
	sb, err := ffiwrapper.New(sp)
	if err != nil {
		panic(err)
	}

	return &SectorSealer{
		sp: sp,
		sb: sb,
	}
}

func (ssb *SectorSealer) GetParams(ctx context.Context,s abi.SectorSize)  {
	dat, err := ioutil.ReadFile("parameters.json")
	if err != nil {
		panic(err)
	}

	datSrs, err := ioutil.ReadFile("srs-inner-product.json")
	if err != nil {
		panic(err)
	}

	err = paramfetch.GetParams(ctx, dat, datSrs, uint64(s))
	if err != nil {
		panic(xerrors.Errorf("failed to acquire Groth parameters for 2KiB sectors: %w", err))
	}
	ssb.sectorSize = s
}

func (ssb *SectorSealer) ExistingPiecesSize(toIndex int) abi.UnpaddedPieceSize {
	existingPiecesSize := abi.UnpaddedPieceSize(0)
	if toIndex < 0 {
		toIndex = int(^uint(0) >> 1)
	}
	for i,piece:=range ssb.pieces {
		if i > toIndex {
			break
		}
		existingPiecesSize += piece.Size.Unpadded()
	}

	return existingPiecesSize
}


//TODO: how to add more than one piece
func (ssb *SectorSealer) AddPiece(ctx context.Context, id storage.SectorRef, sz abi.UnpaddedPieceSize,rs io.ReadSeeker) error{
	var (
		pi abi.PieceInfo
		err error
	)
	ssize, err := id.ProofType.SectorSize()

	pinfo,err := getPieceInfo(rs)
	if err != nil {
		return err
	}
	rs.Seek(0, io.SeekStart)

	var offset abi.UnpaddedPieceSize
	pieceSizes := make([]abi.UnpaddedPieceSize, len(ssb.pieces))
	for i, p := range ssb.pieces {
		pieceSizes[i] = p.Size.Unpadded()
		offset += p.Size.Unpadded()
	}

	if ssb.ref != nil {
		if ssb.ref.ID != id.ID || ssb.ref.ProofType != id.ProofType {
			return xerrors.New("not matching sector ref")
		}
	} else {
		ssb.ref = &id
	}


	if len(ssb.pieces) == 0 {
		ssb.pieces = make([]abi.PieceInfo, 0)
	}

	//existingPiecesSize := ssb.ExistingPiecesSize(-1)
	pads, padLength := ffiwrapper.GetRequiredPadding(offset.Padded(), sz.Padded())
	if offset.Padded()+padLength+sz.Padded() > abi.PaddedPieceSize(ssize) {
		return xerrors.New("exceeding max sector-size")
	}

	for _, p := range pads {
		_, err := ssb.sb.AddPiece(ctx,
			*ssb.ref,
			pieceSizes,
			p.Unpadded(),
			sealing.NewNullReader(p.Unpadded()))
		if err != nil {
			return xerrors.Errorf("writing padding piece: %w", err)
		}

		pieceSizes = append(pieceSizes, p.Unpadded())
	}

	//if existingPiecesSize == 0 {
	//	pi, err = ssb.sb.AddPiece(ctx, id, []abi.UnpaddedPieceSize{}, sz, rs)
	//} else {
	//	pi, err = ssb.sb.AddPiece(ctx, id, []abi.UnpaddedPieceSize{existingPiecesSize}, sz, rs)
	//}
	//if err != nil {
	//	return err
	//}

	piz := pinfo.Size
	paddedReader, err := shared.NewInflatorReader(rs, uint64(sz), piz)
	if err != nil {
		return err
	}
	pi, err = ssb.sb.AddPiece(ctx, id, pieceSizes, pinfo.Size, paddedReader)
	if err != nil {
		return err
	}

	ssb.dataSizes = append(ssb.dataSizes, sz)
	ssb.pieces = append(ssb.pieces, pi)



	return nil
}

func (ssb *SectorSealer) Pack(ctx context.Context) error{
	var allocated abi.UnpaddedPieceSize
	for _, piece := range ssb.pieces{
		allocated += piece.Size.Unpadded()
	}

	ssize, err := ssb.ref.ProofType.SectorSize()
	if err != nil {
		return err
	}

	ubytes := abi.PaddedPieceSize(ssize).Unpadded()

	fillerSizes, err := fillersFromRem(ubytes - allocated)
	if err != nil {
		return err
	}
	sizes:=fillerSizes

	if len(sizes) == 0 {
		return nil
	}

	existingPiecesSize := ssb.ExistingPiecesSize(-1)
	for _, size := range sizes {
		ppi, err := ssb.sb.AddPiece(ctx, *ssb.ref, []abi.UnpaddedPieceSize{existingPiecesSize}, size, sealing.NewNullReader(size))
		if err != nil {
			return xerrors.Errorf("add piece: %w", err)
		}
		ssb.pieces = append(ssb.pieces, ppi)
	}

	return nil
}

func (ssb *SectorSealer) ReadPiece(ctx context.Context,writer io.Writer, id storage.SectorRef,index int) (bool, error){
	pi,err := ssb.getPiece(index)
	if err != nil {
		return false,err
	}

	return ssb.sb.ReadPiece(ctx, writer,id,storiface.UnpaddedByteIndex(ssb.ExistingPiecesSize(index)),pi.Size.Unpadded())
}

func (ssb *SectorSealer) PreCommit(ctx context.Context, ticket abi.SealRandomness) (err error) {
	ssb.ticket = ticket

	err = ssb.preCommit1(ctx, *ssb.ref,ticket)
	if err != nil {
		return err
	}

	err = ssb.preCommit2(ctx, *ssb.ref)
	if err != nil {
		return err
	}

	return nil
}

func (ssb *SectorSealer) preCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness) (err error) {
	ssb.p1, err = ssb.sb.SealPreCommit1(ctx,sector,ticket,ssb.pieces)
	return
}

func (ssb *SectorSealer) preCommit2(ctx context.Context, sector storage.SectorRef) (err error) {
	ssb.cids, err = ssb.sb.SealPreCommit2(context.TODO(), sector, ssb.p1)
	return
}

func (ssb *SectorSealer) Commit(ctx context.Context, seed abi.InteractiveSealRandomness) (err error) {
	err = ssb.commit1(ctx,seed)
	if err != nil {
		return err
	}

	err = ssb.commit2(ctx)
	if err != nil {
		return err
	}

	ok, err := ffiwrapper.ProofVerifier.VerifySeal(prooftypes.SealVerifyInfo{
		SectorID:              ssb.ref.ID,
		SealedCID:             ssb.cids.Sealed,
		SealProof:             ssb.ref.ProofType,
		Proof:                 ssb.proof,
		Randomness:            ssb.ticket,
		InteractiveRandomness: seed,
		UnsealedCID:           ssb.cids.Unsealed,
	})
	if err != nil {
		return fmt.Errorf("%+v", err)
	}

	if !ok {
		return fmt.Errorf("proof failed to validate")
	}

	return nil
}

func (ssb *SectorSealer) FinalizeSector(ctx context.Context, keepUnsealed []storage.Range) error {
	return ssb.sb.FinalizeSector(ctx, *ssb.ref,keepUnsealed)
}

func (ssb *SectorSealer) commit1(ctx context.Context, seed abi.InteractiveSealRandomness) (err error) {
	ssb.seed = seed
	ssb.c1,err = ssb.sb.SealCommit1(ctx, *ssb.ref, ssb.ticket ,seed,ssb.pieces,ssb.cids)
	return
}

func (ssb *SectorSealer) commit2(ctx context.Context) (err error) {
	ssb.proof,err = ssb.sb.SealCommit2(ctx,*ssb.ref,ssb.c1)
	return err
}

func (ssb *SectorSealer) RemoveUnsealed(sector storage.SectorRef) (err error) {
	var b bytes.Buffer
	_, err = ssb.sb.ReadPiece(context.TODO(), &b, sector, 0, 1016)
	if err != nil {
		return err
	}

	p, sd, err := ssb.sp.AcquireSector(context.TODO(), sector, storiface.FTUnsealed, storiface.FTNone, storiface.PathStorage)
	if err != nil {
		return err
	}
	if err := os.Remove(p.Unsealed); err != nil {
		return err
	}
	sd()

	return nil
}

func (ssb *SectorSealer) Unseal(ctx context.Context,si storage.SectorRef,index int, done func()) (bz []byte, err error) {
	var b bytes.Buffer
	if err = ssb.sb.UnsealPiece(context.TODO(), si, 0, ssb.pieces[index].Size.Unpadded(), ssb.ticket, ssb.cids.Unsealed); err != nil {
		return []byte{}, err
	}

	_, err = ssb.sb.ReadPiece(context.TODO(), &b, si, 0, ssb.pieces[index].Size.Unpadded())
	if err != nil {
		return []byte{}, err
	}

	dataSize := uint64(ssb.dataSizes[index])
	bz = b.Bytes()[:dataSize]

	return
}

func (ssb *SectorSealer) getPiece(index int) (*abi.PieceInfo,error){
	indexExceedsArrayBoundError := fmt.Errorf("index exceeds array bound")

	if len(ssb.pieces) == 0 {
		return nil,indexExceedsArrayBoundError
	}

	if len(ssb.pieces) <= index {
		return nil,indexExceedsArrayBoundError
	}

	return &ssb.pieces[index],nil
}

func fillersFromRem(in abi.UnpaddedPieceSize) ([]abi.UnpaddedPieceSize, error) {
	toFill := uint64(in + (in / 127))
	out := make([]abi.UnpaddedPieceSize, bits.OnesCount64(toFill))
	for i := range out {
		// Extract the next lowest non-zero bit
		next := bits.TrailingZeros64(toFill)
		psize := uint64(1) << next

		toFill ^= psize

		// Add the piece size to the list of pieces we need to create
		out[i] = abi.PaddedPieceSize(psize).Unpadded()
	}
	return out, nil
}

func getPieceInfo(rdr io.Reader) (*api.CommPRet, error) {
	w := &writer.Writer{}
	_, err := io.CopyBuffer(w, rdr, make([]byte, writer.CommPBuf))
	if err != nil {
		return nil, xerrors.Errorf("copy into commp writer: %w", err)
	}

	commp, err := w.Sum()
	if err != nil {
		return nil, xerrors.Errorf("computing commP failed: %w", err)
	}

	return &api.CommPRet{
		Root: commp.PieceCID,
		Size: commp.PieceSize.Unpadded(),
	}, nil
}