package chartDownloader

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	backoff "github.com/cenkalti/backoff"
	humanize "github.com/dustin/go-humanize"
)

// A chart in chartmuseum looks like this...
//{
//    "name": "dex",
//   "home": "https://github.com/coreos/dex/",
//   "version": "0.1.1",
//   "description": "CoreOS Dex",
//   "keywords": [
//     "dex",
//     "oidc"
//   ],
//   "maintainers": [
//     {
//       "name": "kfox1111",
//       "email": "Kevin.Fox@pnnl.gov"
//     },
//     {
//       "name": "sstarcher",
//       "email": "shane.starcher@gmail.com"
//     }
//   ],
//   "icon": "https://github.com/coreos/dex/raw/master/Documentation/logos/dex-glyph-color.png",
//   "appVersion": "2.10.0",
//   "urls": [
//     "charts/dex-0.1.1.tgz"
//   ],
//   "created": "2018-04-03T14:21:11.699507024Z",
//   "digest": "34ec5deb42e6d9550ce1416f4f4bf20abd8f9b77110d4c7cfb70a30b38553c3f"
// },

// Chart is a struct representing a chartmuseum chart in the manifest
type Chart struct {
	Name        string       `json:"name"`
	Home        string       `json:"home"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Keywords    []string     `json:"keywords"`
	Maintainers []Maintainer `json:"maintainers"`
	Icon        string       `json:"icon"`
	AppVersion  string       `json:"appVersion"`
	URLS        []string     `json:"urls"`
	Created     string       `json:"created"`
	Digest      string       `json:"digest"`
}

// Maintainer is a struct representing a maintainer inside a chart
type Maintainer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// WriteCounter counts the number of bytes written to it. It implements to the io.Writer
// interface and we can pass this into io.TeeReader() which will report progress on each
// write cycle.
type WriteCounter struct {
	Total uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

// PrintProgress Outputs the status of the download
func (wc WriteCounter) PrintProgress() {
	// Clear the line by using a character return to go back to the start and remove
	// the remaining characters by filling it with spaces
	fmt.Printf("\r%s", strings.Repeat(" ", 35))

	// Return again and print current status of download
	// We use the humanize package to print the bytes in a meaningful way (e.g. 10 MB)
	fmt.Printf("\rDownloading... %s complete", humanize.Bytes(wc.Total))
}

// Run executes the download
func Run(chartmuseumAddress string) {
	if chartmuseumAddress == "" {
		fmt.Println("You must enter a url for the chartmuseum server you want to download from using the --url flag")
		os.Exit(1)
	}

	fmt.Println("Checking for charts...")

	res, err := http.Get(chartmuseumAddress + "/api/charts")
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
	defer res.Body.Close()
	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}

	data := map[string][]Chart{}

	if err := json.Unmarshal([]byte(contents), &data); err != nil {
		panic(err)
	}

	urls := []string{}

	for _, v := range data {

		for _, x := range v {
			for _, u := range x.URLS {
				urls = append(urls, u)
			}
		}
	}

	fmt.Println("Download Started")
	if _, err := os.Stat("charts"); os.IsNotExist(err) {
		os.Mkdir("charts", 0777)
	}
	for _, url := range urls {
		fileName := strings.TrimPrefix(url, "charts/")
		if _, err := os.Stat("charts/" + fileName); os.IsNotExist(err) {

			f := func() error {
				err := DownloadFile("charts/"+fileName, chartmuseumAddress+"/"+url)
				if err != nil {
					return err
				}
				return nil
			}
			exponentialBackOff := backoff.NewExponentialBackOff()
			exponentialBackOff.MaxElapsedTime = 30 * time.Second
			exponentialBackOff.Reset()
			err := backoff.Retry(f, exponentialBackOff)
			if err != nil {
				panic(err)
			}

		}
	}

	fmt.Println("Download Finished")
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory. We pass an io.TeeReader
// into Copy() to report progress on the download.
func DownloadFile(filepath string, url string) error {

	// Create the file, but give it a tmp file extension, this means we won't overwrite a
	// file until it's downloaded, but we'll remove the tmp extension once downloaded.
	out, err := os.Create(filepath + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create our progress reporter and pass it to be used alongside our writer
	counter := &WriteCounter{}
	_, err = io.Copy(out, io.TeeReader(resp.Body, counter))
	if err != nil {
		return err
	}

	// The progress use the same line so print a new line once it's finished downloading
	fmt.Print("\n")

	err = os.Rename(filepath+".tmp", filepath)
	if err != nil {
		return err
	}

	return nil
}
