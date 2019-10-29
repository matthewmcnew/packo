package upload

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"io/ioutil"
	"os"
)

func Upload(path, registy string) (string, error) {
	image, err := random.Image(0, 0)
	if err != nil {
		return "", err
	}

	tempDir, err := ioutil.TempDir("", "upload")
	defer os.RemoveAll(tempDir)
	if err != nil {
		return "", err
	}

	layer, err := tarball.LayerFromReader(ReadDirAsTar(path, "/", 0, 0, -1))
	if err != nil {
		return "", err
	}

	image, err = mutate.AppendLayers(image, layer)
	if err != nil {
		return "", err
	}
	ref, err := name.ParseReference(registy + "/source")
	if err != nil {
		return "", err
	}

	err = remote.Write(ref, image, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", err
	}

	hash, err := image.Digest()
	return registy + "/source@" + hash.String(), err
}
