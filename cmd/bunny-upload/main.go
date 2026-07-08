package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/etkecc/go-kit/httpclient"
	"github.com/etkecc/go-kit/workpool"

	"github.com/etkecc/bunny-upload/internal/config"
)

const (
	// CDN API endpoint
	CDNAPI = "https://api.bunny.net"
	// StorageAPI endpoint
	StorageAPI = "https://storage.bunnycdn.com"
)

var (
	configPath string
	wp         *workpool.WorkPool
	client     *http.Client
	cfg        *config.Config
)

func main() {
	if err := parseConfig(); err != nil {
		panic(err)
	}

	wp = workpool.New(75) // max concurrent connections to storage zone
	// HTTP/1 only: Bunny storage sends GOAWAY mid-upload on HTTP/2, and a multi-GB retry loop is not the hill to die on.
	// HTTP2 stays explicitly off, not just unset: WithProtocols replaces the preset's H1+H2 default wholesale.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(false)
	client = httpclient.NewSingleHost(httpclient.WithProtocols(protocols))

	if err := filepath.Walk(cfg.Path, walkfs); err != nil {
		panic(err)
	}

	wp.Run()

	if err := purgeCache(); err != nil {
		panic(err)
	}
}

func parseConfig() error {
	flag.StringVar(&configPath, "c", "", "path to the config file")
	flag.Parse()

	var err error
	cfg, err = config.Read(configPath)
	if err != nil {
		return err
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
	wp.Do(func() {
		if err := uploadFile(uri, path, rpath); err != nil {
			fmt.Printf("%s uri=%s status=FAIL err=%v\n", rpath, uri, err)
		}
	})

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
