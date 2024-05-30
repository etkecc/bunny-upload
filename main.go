package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/xxjwxc/gowp/workpool"
	"gitlab.com/etke.cc/tools/bunny-upload/config"
)

const (
	// CDN API endpoint
	CDNAPI = "https://api.bunny.net"
	// StorageAPI endpoint
	StorageAPI = "https://storage.bunnycdn.com"
)

var (
	configPath        string
	pullZoneID        int
	pullZoneAccessKey string
	storageZone       string
	storagePassword   string
	wp                *workpool.WorkPool
	client            *http.Client
	cfg               *config.Config
)

func main() {
	if err := parseConfig(); err != nil {
		panic(err)
	}

	wp = workpool.New(75) // max concurrent connections to storage zone
	client = &http.Client{
		Transport: &http.Transport{
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, // disable HTTP2 due to GOAWAY issue
		},
	}

	if err := filepath.Walk(cfg.Path, walkfs); err != nil {
		panic(err)
	}

	if err := wp.Wait(); err != nil {
		panic(err)
	}

	if err := purgeCache(); err != nil {
		panic(err)
	}
}

func parseConfig() error {
	var argpath string
	flag.StringVar(&configPath, "c", "", "path to the config file")

	flag.StringVar(&argpath, "p", "", "path to the folder to upload recursively")

	flag.StringVar(&storageZone, "z", "", "storage zone")
	flag.StringVar(&storagePassword, "k", "", "access key (storage zone password)")

	flag.IntVar(&pullZoneID, "i", 0, "pull zone ID")
	flag.StringVar(&pullZoneAccessKey, "a", "", "access key (pull zone)")

	flag.Parse()

	if configPath == "" {
		cfg = &config.Config{
			Path: argpath,
			Cache: config.Cache{
				PullZone:  pullZoneID,
				AccessKey: pullZoneAccessKey,
			},
			Storage: config.Storage{
				Zone:     storageZone,
				Password: storagePassword,
			},
		}
	} else {
		var err error
		cfg, err = config.Read(configPath)
		if err != nil {
			return err
		}
	}

	absPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return err
	}
	cfg.Path = absPath
	return nil
}

func getURL(path string) (url, uri string) {
	rpath := strings.TrimPrefix(path, cfg.Path+"/")
	return fmt.Sprintf("%s/%s/%s", StorageAPI, cfg.Storage.Zone, rpath), rpath
}

func uploadFile(uri, path, rpath string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the first 512 bytes for content type detection, neede for http.DetectContentType
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	contentType := http.DetectContentType(buffer[:n])

	// Reset the file pointer to the beginning
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, uri, file)
	if err != nil {
		return err
	}
	defer req.Body.Close()

	req.Header.Add("AccessKey", cfg.Storage.Password)
	req.Header.Add("Content-Type", contentType)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		body, err := io.ReadAll(resp.Body)
		fmt.Printf("%s uri=%s status=FAIL code=%d %s err=%v\n", rpath, uri, resp.StatusCode, string(body), err)
	} else {
		fmt.Printf("%s status=OK\n", rpath)
	}

	return nil
}

func walkfs(path string, info fs.FileInfo, _ error) error {
	if info.IsDir() {
		return nil
	}
	uri, rpath := getURL(path)
	wp.Do(func() error { return uploadFile(uri, path, rpath) })

	return nil
}

func purgeCache() error {
	if cfg.Cache.PullZone == 0 || cfg.Cache.AccessKey == "" {
		fmt.Println("no pull zone or access key specified, skipping cache purge")
		return nil
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/pullzone/%d/purgeCache", CDNAPI, cfg.Cache.PullZone), http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AccessKey", cfg.Cache.AccessKey)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		body, err := io.ReadAll(resp.Body)
		return fmt.Errorf("purge cache failed: %d %s %w", resp.StatusCode, string(body), err)
	}
	fmt.Println("cache purged")
	return nil
}
