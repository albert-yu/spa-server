package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

// spaHandler implements the http.Handler interface, so we can use it
// to respond to HTTP requests. The path to the static directory and
// path to the index file within that static directory are used to
// serve the SPA in the given static directory.
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ServeHTTP inspects the URL path to locate a file within the static dir
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		// and stop
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(h.staticPath, path)

	// check whether a file exists at the given path
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

// CmdLineArgs is a struct containing
// the parsed command line arguments
type CmdLineArgs struct {
	Host      string
	Port      int
	RootDir   string
	Wait      time.Duration
	SSL       bool
	FullChain string
	PrivKey   string
}

func parseArgs() CmdLineArgs {
	var args CmdLineArgs
	flag.IntVar(
		&args.Port,
		"port",
		5000,
		"Specify the port this app should listen on for requests",
	)
	flag.StringVar(
		&args.Host,
		"host",
		"0.0.0.0",
		"Specify the host of this service",
	)
	flag.StringVar(
		&args.RootDir,
		"rootdir",
		"./",
		"The folder where we should serve the SPA, usually where index.html is located",
	)
	flag.DurationVar(
		&args.Wait,
		"graceful-timeout",
		time.Second*15,
		"The duration for which the server should gracefully wait for existing connections to finish",
	)
	flag.BoolVar(
		&args.SSL,
		"ssl",
		false,
		"Run in SSL mode?",
	)
	flag.StringVar(
		&args.FullChain,
		"fullchain",
		"",
		"Path to fullchain.pem",
	)
	flag.StringVar(
		&args.PrivKey,
		"privkey",
		"",
		"Path to privkey.pem",
	)
	flag.Parse()
	return args
}

func main() {
	args := parseArgs()

	// web server
	const writeTimeout = 1 * 60
	const readTimeout = 1 * 60
	const idleTimeout = 2 * 60
	addr := fmt.Sprintf("%s:%d", args.Host, args.Port)
	if args.SSL {
		if args.Port != 443 {
			log.Fatal("Port needs to be 443 if SSL enabled")
		}
		if args.FullChain == "" {
			log.Fatal("Path to fullchain.pem required if SSL enabled")
		}
		if args.PrivKey == "" {
			log.Fatal("Path to privkey.pem required if SSL enabled")
		}
	}

	r := mux.NewRouter()

	// ping for convenience
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{\"response\": \"pong\"}"))
	}).Methods("GET")

	spa := spaHandler{
		staticPath: args.RootDir,
		indexPath:  "index.html",
	}
	r.PathPrefix("/").Handler(spa)

	handler := cors.Default().Handler(r)
	srv := &http.Server{
		Handler:      handler,
		Addr:         addr,
		WriteTimeout: writeTimeout * time.Second,
		ReadTimeout:  readTimeout * time.Second,
		IdleTimeout:  idleTimeout * time.Second,
	}
	log.Println("Listening on", addr)
	log.Println("Press Ctrl+C to quit")

	// run in goroutine to avoid blocking
	if args.SSL {
		go func() {
			if err := srv.ListenAndServeTLS(
				args.FullChain,
				args.PrivKey,
			); err != nil {
				log.Println(err)
			}
		}()
	} else {
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				log.Println(err)
			}
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// block until we receive our signal
	<-c
	ctx, cancel := context.WithTimeout(context.Background(), args.Wait)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("Shutting down...")
	os.Exit(0)
}
