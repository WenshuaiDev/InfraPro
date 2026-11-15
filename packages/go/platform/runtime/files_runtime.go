package runtime

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
)

func (b Business) configureFiles(o config.Files) {
	if b.Files == nil {
		return
	}
	if o.MaxFileBytes == 0 {
		o = config.DefaultFiles()
	}
	b.Files.WithOptions(o).WithChecker(files.SocketChecker{Socket: o.CheckerSocket, Timeout: o.CheckerTimeout})
	if o.ObjectEndpoint != "" {
		objects, e := blobstore.Open(o.ObjectEndpoint, o.ObjectAccessKey, o.ObjectSecretKey, o.Bucket, "files/v1")
		if e != nil {
			panic(e)
		}
		b.Files.WithStorage(objects)
	}
}
