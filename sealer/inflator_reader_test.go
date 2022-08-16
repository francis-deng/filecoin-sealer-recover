package sealer

import (
	"bytes"
	"github.com/filecoin-project/go-fil-markets/shared"
	"testing"
)

func TestInflatorReader(t *testing.T) {
	rs := bytes.NewReader([]byte(francis))
	paddedReader, err := shared.NewInflatorReader(rs, uint64(130), 254)
	if err != nil {
		t.Fatal(err)
	}

	buff := make([]byte, 256)
	n,err := paddedReader.Read(buff)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(n)
}
