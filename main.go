package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/foomo/simplecert"
	"github.com/foomo/tlsconfig"
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
	Domain    string
	SSL       bool
	CertCache string
	SSLEmail  string
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
	flag.StringVar(
		&args.Domain,
		"domain",
		"",
		"The public domain name of the site",
	)
	flag.BoolVar(
		&args.SSL,
		"ssl",
		false,
		"Run in SSL mode?",
	)
	flag.StringVar(
		&args.CertCache,
		"certcache",
		"",
		"Path to the certificate cache (e.g. letsencrypt/live/mysite.com/)",
	)
	flag.StringVar(
		&args.SSLEmail,
		"sslemail",
		"",
		"SSL email address",
	)
	flag.Parse()
	return args
}

func certAndKey(certCache string) (string, string) {
	return path.Join(certCache, "cert.pem"), path.Join(certCache, "key.pem")
}

func serveTLS(srv *http.Server, certCache string) {
	cert, key := certAndKey(certCache)
	go func() {
		if err := srv.ListenAndServeTLS(cert, key); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %+s\n", err)
		}
	}()
}

func main() {
	args := parseArgs()

	// web server
	const (
		writeTimeout = 1 * 60
		readTimeout  = 1 * 60
		idleTimeout  = 2 * 60
	)

	if args.SSL {
		if args.Port != 443 {
			args.Port = 443
			log.Println("Port set to 443 since SSL enabled")
		}
		if args.CertCache == "" {
			log.Fatal("Path certificate cache required if SSL enabled")
		}
		if args.SSLEmail == "" {
			log.Fatal("SSL Email if SSL enabled")
		}
	}
	addr := fmt.Sprintf("%s:%d", args.Host, args.Port)

	makeServer := func(rootDir, addr string) *http.Server {
		r := mux.NewRouter()

		// ping for convenience
		r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("{\"response\": \"pong\"}"))
		}).Methods("GET")

		spa := spaHandler{
			staticPath: rootDir,
			indexPath:  "index.html",
		}
		r.PathPrefix("/").Handler(spa)

		handler := cors.Default().Handler(r)
		return &http.Server{
			Handler:      handler,
			Addr:         addr,
			WriteTimeout: writeTimeout * time.Second,
			ReadTimeout:  readTimeout * time.Second,
			IdleTimeout:  idleTimeout * time.Second,
		}
	}

	srv := makeServer(args.RootDir, addr)

	// run in goroutine to avoid blocking
	ctx, cancel := context.WithTimeout(context.Background(), args.Wait)
	defer cancel()
	if args.SSL {
		var (
			certReloader *simplecert.CertReloader
			numRenews    int
			cfg          = simplecert.Default
			tlsConf      = tlsconfig.NewServerTLSConfig(tlsconfig.TLSModeServerStrict)
		)

		cfg.Domains = []string{args.Domain}
		cfg.CacheDir = args.CertCache
		cfg.SSLEmail = args.SSLEmail
		cfg.HTTPAddress = ""

		cfg.WillRenewCertificate = func() {
			cancel()
		}

		cfg.DidRenewCertificate = func() {
			numRenews++
			srv = makeServer(args.RootDir, addr)
			srv.TLSConfig = tlsConf

			certReloader.ReloadNow()

			serveTLS(srv, args.CertCache)
		}

		certReloader, err := simplecert.Init(cfg, func() {
			os.Exit(0)
		})
		if err != nil {
			log.Fatal("simplecert init failed: ", err)
		}

		// redirect to HTTPS
		go http.ListenAndServe(":80", http.HandlerFunc(simplecert.Redirect))

		// enable hot reload
		tlsConf.GetCertificate = certReloader.GetCertificateFunc()

		serveTLS(srv, args.CertCache)
	} else {
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				log.Println(err)
			}
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)

	// block until we receive our signal
	<-c
	err := srv.Shutdown(ctx)

	log.Println("Shutting down...")
	if err == http.ErrServerClosed {
		log.Println("Server exited properly")
	} else if err != nil {
		log.Println("Unexpected error on exit:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
