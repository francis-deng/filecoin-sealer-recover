package storage

import (
	"fmt"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"os"
	"path/filepath"
)

var log = logging.Logger("storage")

var StorageCmd = &cli.Command{
	Name:      "storage",
	Usage:     "build sector storage",
	ArgsUsage: "[storage root path]",
	Flags: []cli.Flag{
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("sector storage must be specified")
		}

		storageRoot := cctx.Args().First()

		if err := os.Mkdir(filepath.Join(storageRoot, storiface.FTUnsealed.String()), 0755); err != nil && !os.IsExist(err) { // nolint
			return err
		}
		if err := os.Mkdir(filepath.Join(storageRoot, storiface.FTSealed.String()), 0755); err != nil && !os.IsExist(err) { // nolint
			return err
		}
		if err := os.Mkdir(filepath.Join(storageRoot, storiface.FTCache.String()), 0755); err != nil && !os.IsExist(err) { // nolint
			return err
		}
		if err := os.Mkdir(filepath.Join(storageRoot, storiface.FTUpdate.String()), 0755); err != nil && !os.IsExist(err) { // nolint
			return err
		}
		if err := os.Mkdir(filepath.Join(storageRoot, storiface.FTUpdateCache.String()), 0755); err != nil && !os.IsExist(err) { // nolint
			return err
		}

		return nil
	},
}


