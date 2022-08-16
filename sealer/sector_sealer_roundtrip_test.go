package sealer

import (
	"context"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"testing"
)

var sealRand = abi.SealRandomness{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2}
var seed = abi.InteractiveSealRandomness{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

var (
	francis = "Francis Deng is writer, scholar, diplomat and South Sudan's first ambassador to the United Nations. From 2006 to 2007, Mr. Deng Jr"
)

func TestAddAndReadPiece(t *testing.T) {
	var sealProofType = abi.RegisteredSealProof_StackedDrg2KiBV1
	var sectorSize, _ = sealProofType.SectorSize()
	//var actorReaders []io.ReadSeeker
	//var actorNameSizes []int

	root, _ := ioutil.TempDir(os.TempDir(), "ssealer.")
	t.Log("TempDir",root)


	//for _,actor:=range []string{
	//	francis} {
	//	//r := strings.NewReader(actor)
	//	rs := bytes.NewReader([]byte(actor))
	//	actorReaders = append(actorReaders, rs)
	//	actorNameSizes = append(actorNameSizes, len([]byte(actor)))
	//}

	ss := New(root)
	defer os.RemoveAll(root)

	ctx := context.Background()

	ss.GetParams(ctx, sectorSize)

	miner := abi.ActorID(123)
	si := storage.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	//for i,r:=range actorReaders {
	//	err := ss.AddPiece(ctx, si, abi.UnpaddedPieceSize(actorNameSizes[i]),r)
	//	if err!= nil {
	//		t.Error(err)
	//		return
	//	}
	//}
	f,err := os.Open("us.txt")
	if err != nil {
		panic(err)
	}
	fi,_ := f.Stat()
	t.Log("file-size",fi.Size())
	defer f.Close()

	srcFileBytes, err := ioutil.ReadAll(f)
	srcFileContent := string(srcFileBytes)
	t.Log("retrieval content:", srcFileContent)

	err = ss.AddPiece(ctx, si, abi.UnpaddedPieceSize(fi.Size()),f)
	if err!= nil {
		t.Error(err)
		return
	}

	err = ss.Pack(ctx)
	if err != nil {
		panic(err)
	}

	err = ss.PreCommit(ctx, sealRand)
	if err != nil {
		panic(err)
	}

	err = ss.Commit(ctx,seed)
	if err != nil {
		panic(err)
	}

	err = ss.FinalizeSector(ctx,nil)
	if err != nil {
		panic(err)
	}

	err = ss.RemoveUnsealed(si)
	if err != nil {
		panic(err)
	}

	bz,err := ss.Unseal(ctx, si, 0, func(){})
	if err != nil {
		panic(err)
	}

	t.Log("unsealed content: ", string(bz))
	require.Equal(t, srcFileBytes, bz)
}
