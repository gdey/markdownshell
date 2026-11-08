package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdey/markdownshell/notebook"
)

// The rendered view doesn't load images from other sites on its own:
// drawing one would tell whoever serves it that the document was opened.
// The note drawn in its place has a button that loads it when the user
// asks, through RemoteImage, which remembers the answer for that image in
// that document. The window's CSP still allows no remote images, so the
// image comes back as a data: URI.

// maxRemoteImage is the largest image LoadRemoteImage takes.
const maxRemoteImage = 20 << 20

// remoteImageTypes are the image types LoadRemoteImage hands the window,
// as http.DetectContentType names them from the image's first bytes.
// SVG isn't among them: it is a document that can hold links and
// scripts, and nothing in its bytes says what it will do.
var remoteImageTypes = map[string]bool{
	"image/png":                true,
	"image/jpeg":               true,
	"image/gif":                true,
	"image/webp":               true,
	"image/bmp":                true,
	"image/x-icon":             true,
	"image/vnd.microsoft.icon": true,
}

// remoteImageClient fetches images the user asked for, following at
// most a few redirects, and only to http: and https: addresses.
var remoteImageClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirected to a %s: address", req.URL.Scheme)
		}
		return nil
	},
}

// RemoteImage returns the image at src, an image in this window's
// document that the window doesn't load by itself, as a data: URI for it
// to show: an http: or https: address on another site, or a file outside
// the document's folder (or hidden in it), which the window's /doc route
// doesn't serve. With load false, it only does so for an
// image the user loaded in this document before, and returns "" for any
// other, without going to the network. With load true, from the note's
// Load button, it fetches the image, and remembers that this document
// may load it, so it loads by itself from then on. Anything that isn't
// a PNG, JPEG, GIF, WebP, BMP or icon, or is over 20 MB, is refused.
func (a *App) RemoteImage(ctx context.Context, src string, load bool) (string, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	file := n.File()
	if !load && !a.images.Has(file, src) {
		return "", nil
	}
	var uri string
	if path, ok := notebook.OutsideImage(file, src); ok {
		// A file is read again each time, so a changed one shows as
		// it is now.
		if uri, err = loadImageFile(path); err != nil {
			return "", err
		}
	} else {
		var ok bool
		if uri, ok = a.remote.get(src); !ok {
			if uri, err = loadRemoteImage(ctx, remoteImageClient, src); err != nil {
				return "", err
			}
			a.remote.put(src, uri)
		}
	}
	if load {
		if err := a.images.Add(file, src); err != nil {
			return uri, fmt.Errorf("loaded, but couldn't remember it for next time: %w", err)
		}
	}
	return uri, nil
}

// remoteCache keeps the last few images RemoteImage fetched.
type remoteCache struct {
	mu   sync.Mutex
	keys []string
	uris map[string]string
}

// maxRemoteCache is how many images remoteCache keeps.
const maxRemoteCache = 32

func (c *remoteCache) get(src string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	uri, ok := c.uris[src]
	return uri, ok
}

func (c *remoteCache) put(src, uri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.uris == nil {
		c.uris = map[string]string{}
	}
	if _, ok := c.uris[src]; !ok {
		c.keys = append(c.keys, src)
	}
	c.uris[src] = uri
	for len(c.keys) > maxRemoteCache {
		delete(c.uris, c.keys[0])
		c.keys = c.keys[1:]
	}
}

// loadImageFile reads the image at path, by the same rules as
// loadRemoteImage: an image type by its bytes, and no more than 20 MB.
func loadImageFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("couldn't load the image: %w", err)
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("couldn't load the image: %s isn't a file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxRemoteImage+1))
	if err != nil {
		return "", fmt.Errorf("couldn't load the image: %w", err)
	}
	return imageURI(data, path)
}

func loadRemoteImage(ctx context.Context, client *http.Client, src string) (string, error) {
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("mdsh-gui only loads http: and https: images, not %q", src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/webp,image/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't load the image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("couldn't load the image: %s answered %s", u.Host, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteImage+1))
	if err != nil {
		return "", fmt.Errorf("couldn't load the image: %w", err)
	}
	return imageURI(data, u.Host)
}

// imageURI is data as a data: URI, if it is an image of one of
// remoteImageTypes, by its bytes, and no more than maxRemoteImage. from
// says where it came from, for the error.
func imageURI(data []byte, from string) (string, error) {
	if len(data) > maxRemoteImage {
		return "", fmt.Errorf("the image is over %d MB", maxRemoteImage>>20)
	}
	typ, _, _ := strings.Cut(http.DetectContentType(data), ";")
	if !remoteImageTypes[typ] {
		return "", fmt.Errorf("%s isn't an image mdsh-gui shows (it is %s)", from, typ)
	}
	return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
