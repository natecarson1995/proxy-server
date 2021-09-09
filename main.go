package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/joho/godotenv/autoload"
)

func GetOriginalHostReader(method string, path string) (io.ReadCloser, error) {
	// This would be where we can configure if we want the cache to follow redirects or not
	client := &http.Client{}
	// We want a new unique request copying only original method and path information
	proxyRequest, err := http.NewRequest(method, path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request to original host: %s", err)
	}

	proxyResponse, err := client.Do(proxyRequest)
	if err != nil {
		return nil, fmt.Errorf("error getting original host response: %s", err)
	}

	return proxyResponse.Body, nil
}

func GeneralProxy(ctx *gin.Context, path string) {
	// We hash the path here, so that each unique requested path leads to a unique local path
	// Without having to deal with directories or escaping characters
	req := ctx.Request
	hostPath := ctx.Params.ByName("path")

	// Here we get the reader for the original host's file
	originalHostPath := path + hostPath
	hostFileReader, err := GetOriginalHostReader(req.Method, originalHostPath)
	if err != nil {
		log.Fatalf("Error getting original host file: %s", err)
	}
	defer hostFileReader.Close()

	// Do the streaming of the file to the client
	ctx.Stream(func(w io.Writer) bool {
		_, err := io.Copy(w, hostFileReader)
		return err != nil
	})
}
func main() {
	// This is an initial superficial check of the original host url we read files from
	originalHost, err := url.Parse(os.Getenv("ORIGINAL_HOST"))
	if err != nil {
		log.Fatalf("Error parsing original host URL: %s", err)
	}
	cacheDir := os.Getenv("CACHE_DIR")

	router := gin.Default()

	router.GET("/*path", func(ctx *gin.Context) {
		// We hash the path here, so that each unique requested path leads to a unique local path
		// Without having to deal with directories or escaping characters
		req := ctx.Request
		hostPath := ctx.Params.ByName("path")
		localPath := fmt.Sprintf("%x", md5.Sum([]byte(hostPath)))

		// Attempt to open a file reader on the cached file
		var fileReader io.Reader
		file, err := os.Open(cacheDir + localPath)

		if err != nil {
			// This indicates that the file does not exist, and needs to be streamed to the client and cached
			localFile, err := os.Create(cacheDir + localPath)
			if err != nil {
				log.Fatalf("Error creating local file: %s", err)
			}

			// Here we get the reader for the original host's file
			originalHostPath := originalHost.String() + hostPath
			hostFileReader, err := GetOriginalHostReader(req.Method, originalHostPath)
			if err != nil {
				log.Fatalf("Error getting original host file: %s", err)
			}
			defer hostFileReader.Close()

			// Then we inject a Tee in the middle that will also write the file to the cache
			teedReader := io.TeeReader(hostFileReader, localFile)
			fileReader = teedReader
		} else {
			fileReader = file
		}

		// Do the streaming of the file to the client
		ctx.Stream(func(w io.Writer) bool {
			_, err := io.Copy(w, fileReader)
			return err != nil
		})
	})

	router.POST("/*path", func(c *gin.Context) {
		GeneralProxy(c, originalHost.String())
	})
	router.PUT("/*path", func(c *gin.Context) {
		GeneralProxy(c, originalHost.String())
	})
	router.DELETE("/*path", func(c *gin.Context) {
		GeneralProxy(c, originalHost.String())
	})

	router.Run()
}
