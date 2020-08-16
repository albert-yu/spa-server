package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

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

	// static file server
	r.PathPrefix("/").Handler(
		http.FileServer(http.Dir(args.RootDir)))

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
