package sealer

import (
	"context"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"io/ioutil"
	"os"
	"testing"
)


func TestCreateSector(t *testing.T){
	var sealRand = abi.SealRandomness{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2}
	var seed = abi.InteractiveSealRandomness{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

	var sealProofType = abi.RegisteredSealProof_StackedDrg2KiBV1
	var sectorSize, _ = sealProofType.SectorSize()

	var miner = abi.ActorID(123)
	var si = storage.SectorRef{
		ID:        abi.SectorID{Miner: miner, Number: 1},
		ProofType: sealProofType,
	}

	root, _ := ioutil.TempDir(os.TempDir(), "ssealer.")
	t.Log("TempDir: ",root)

	ss := New(root)
	defer os.RemoveAll(root)

	ctx := context.Background()

	t.Run("add piece", func(t *testing.T) {
		ss.GetParams(ctx, sectorSize)

		f,err := os.Open("us2.txt")
		if err != nil {
			panic(err)
		}
		fi,_ := f.Stat()
		t.Log("file-size",fi.Size())
		defer f.Close()

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
	})

}


//func TestInflatorReader(t *testing.T) {
//	rs := bytes.NewReader([]byte(""))
//	paddedReader, err := shared.NewInflatorReader(rs, uint64(130), 254)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	buff := make([]byte, 256)
//	n,err := paddedReader.Read(buff)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	t.Log(n)
//}
