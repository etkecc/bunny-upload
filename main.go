package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/xxjwxc/gowp/workpool"
)

// API endpoint
const API = "https://storage.bunnycdn.com"

var (
	storageZone string
	accessKey   string
	absPath     string
	wp          *workpool.WorkPool
	client      *http.Client
)

func getURL(path string) (string, string) {
	rpath := strings.TrimPrefix(path, absPath+"/")
	return fmt.Sprintf("%s/%s%s", API, storageZone, rpath), rpath
}

func main() {
	var argpath string
	flag.StringVar(&argpath, "p", "", "path to the folder to upload recursively")
	flag.StringVar(&storageZone, "z", "", "storage zone")
	flag.StringVar(&accessKey, "k", "", "access key (storage zone password)")
	flag.Parse()
	wp = workpool.New(75) // max concurrent connections to storage zone
	client = &http.Client{
		Transport: &http.Transport{
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, // disable HTTP2 due to GOAWAY issue
		},
	}

	var err error
	absPath, err = filepath.Abs(argpath)
	if err != nil {
		panic(err)
	}
	err = filepath.Walk(absPath, walkfs)
	if err != nil {
		panic(err)
	}

	err = wp.Wait()
	if err != nil {
		panic(err)
	}
}

func uploadFile(uri, path, rpath string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	req, err := http.NewRequest("PUT", uri, file)
	if err != nil {
		return err
	}

	req.Header.Add("AccessKey", accessKey)
	req.Header.Add("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer req.Body.Close()
	if resp.StatusCode > 299 {
		body, err := io.ReadAll(resp.Body)
		fmt.Printf("%s uri=%s status=FAIL code=%d %s err=%v\n", rpath, uri, resp.StatusCode, string(body), err)
	} else {
		fmt.Printf("%s status=OK\n", rpath)
	}

	return nil
}

func walkfs(path string, info fs.FileInfo, err error) error {
	if info.IsDir() {
		return nil
	}
	uri, rpath := getURL(path)
	wp.Do(func() error { return uploadFile(uri, path, rpath) })

	return nil
}
