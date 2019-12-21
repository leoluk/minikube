package image

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/minikube/pkg/minikube/constants"
	"k8s.io/minikube/pkg/minikube/localpath"
)

// DigestByLocalDaemon uses client by docker lib
func DigestByLocalDaemon(imgClient *client.Client, imgName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	img, _, err := imgClient.ImageInspectWithRaw(ctx, imgName)
	if err != nil && !client.IsErrImageNotFound(err) {
		glog.Infof("couldn't find image digest %s from local daemon: %v ", imgName, err)
		return ""
	}
	return img.ID
}

// DigestByRetrieve gets image digest uses go-containerregistry lib
// which is 4s slower thabn local daemon per lookup https://github.com/google/go-containerregistry/issues/627
func DigestByRetrieve(imgName string) string {
	ref, err := name.ParseReference(imgName, name.WeakValidation)
	if err != nil {
		glog.Infof("error parsing image name %s ref %v ", imgName, err)
		return ""
	}

	img, err := retrieveImage(ref)
	if err != nil {
		glog.Infof("error retrieve Image %s ref %v ", imgName, err)
		return ""
	}

	cf, err := img.ConfigName()
	if err != nil {
		glog.Infof("error getting Image config name %s %v ", imgName, err)
		return cf.Hex
	}
	return cf.Hex
}

func retrieveImage(ref name.Reference) (v1.Image, error) {
	glog.Infof("retrieving image: %+v", ref)
	img, err := daemon.Image(ref)
	if err == nil {
		glog.Infof("found %s locally: %+v", ref.Name(), img)
		return img, nil
	}
	// reference does not exist in the local daemon
	if err != nil {
		glog.Infof("daemon lookup for %+v: %v", ref, err)
	}

	img, err = remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err == nil {
		return img, nil
	}

	glog.Warningf("authn lookup for %+v (trying anon): %+v", ref, err)
	img, err = remote.Image(ref)
	return img, err
}

// DeleteFromImageCacheDir deletes images from the cache
func DeleteFromImageCacheDir(images []string) error {
	for _, image := range images {
		path := filepath.Join(constants.ImageCacheDir, image)
		path = localpath.SanitizeCacheDir(path)
		glog.Infoln("Deleting image in cache at ", path)
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return cleanImageCacheDir()
}

func cleanImageCacheDir() error {
	err := filepath.Walk(constants.ImageCacheDir, func(path string, info os.FileInfo, err error) error {
		// If error is not nil, it's because the path was already deleted and doesn't exist
		// Move on to next path
		if err != nil {
			return nil
		}
		// Check if path is directory
		if !info.IsDir() {
			return nil
		}
		// If directory is empty, delete it
		entries, err := ioutil.ReadDir(path)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err = os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}
