package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.2"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "M365 MCP for Windows\nCommands: login --client-id ID [--tenant ID], serve [--http --port 8000], connection [--http --port 8000], logout, version")
		return nil
	}
	if args[0] == "version" {
		fmt.Println("m365-mcp-windows-local-go " + version)
		return nil
	}
	if args[0] == "extract-worker" {
		return runExtractionWorker()
	}
	if runtime.GOOS != "windows" {
		return errors.New("this application requires Windows 10/11; use the Python container on other platforms")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	clientID := flags.String("client-id", "", "Microsoft Entra public-client application ID")
	tenant := flags.String("tenant", "organizations", "Tenant ID, organizations, or common")
	httpMode := flags.Bool("http", false, "Use authenticated loopback Streamable HTTP instead of stdio")
	port := flags.Int("port", 8000, "Local HTTP port")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *port < 1 || *port > 65535 {
		return errors.New("invalid command arguments or port")
	}
	dir, err := stateDirectory()
	if err != nil {
		return err
	}
	if args[0] == "connection" {
		return printConnection(dir, *httpMode, *port)
	}
	unlock, err := lockState(dir)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	switch args[0] {
	case "login":
		config := localConfig{ClientID: *clientID, Tenant: *tenant}
		if err := config.validate(); err != nil {
			return err
		}
		auth, err := newAuth(dir, config)
		if err != nil {
			return err
		}
		ctx, timeout := context.WithTimeout(ctx, 5*time.Minute)
		defer timeout()
		if err := auth.login(ctx, dir); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Signed in. Run connection to generate your MCP client configuration.")
		return nil
	case "logout":
		for _, name := range []string{"settings.dpapi", "tokens.dpapi"} {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("could not remove the local sign-in cache")
			}
		}
		fmt.Fprintln(os.Stderr, "Local sign-in credentials removed.")
		return nil
	case "serve":
		config, err := loadConfig(dir)
		if err != nil {
			return err
		}
		auth, err := newAuth(dir, config)
		if err != nil {
			return err
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		graph := newGraph(auth.token)
		defer graph.client.CloseIdleConnections()
		downloads := newDownloads()
		server := newMCP(newMailTools(graph, downloads, logger))
		address := "127.0.0.1:0"
		var handler http.Handler
		if *httpMode {
			address = "127.0.0.1:" + strconv.Itoa(*port)
			handler = authenticatedMCP(server, config.HTTPToken)
		}
		listener, err := downloads.start(ctx, address, handler)
		if err != nil {
			return errors.New("could not bind the local listener; check whether the port is already in use")
		}
		defer listener.Close()
		if *httpMode {
			fmt.Fprintln(os.Stderr, "MCP listening at http://"+listener.Addr().String()+"/mcp")
			<-ctx.Done()
			return nil
		}
		return server.Run(ctx, &mcp.StdioTransport{MaxLineLength: 1 << 20})
	default:
		return errors.New("unknown command; use login, serve, connection, logout, or version")
	}
}

func authenticatedMCP(server *mcp.Server, token string) http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(token) != 64 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="m365-mcp-local"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func printConnection(dir string, httpMode bool, port int) error {
	config, err := loadConfig(dir)
	if err != nil {
		return err
	}
	var connection map[string]any
	if httpMode {
		connection = map[string]any{"type": "streamableHttp", "url": fmt.Sprintf("http://127.0.0.1:%d/mcp", port), "headers": map[string]string{"Authorization": "Bearer " + config.HTTPToken}}
	} else {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		connection = map[string]any{"command": executable, "args": []string{"serve"}}
		if root := os.Getenv("M365_ATTACHMENT_ROOT"); root != "" {
			if !filepath.IsAbs(root) {
				return errors.New("M365_ATTACHMENT_ROOT must be an absolute operator-controlled directory")
			}
			connection["env"] = map[string]string{"M365_ATTACHMENT_ROOT": root}
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]any{"mcpServers": map[string]any{"m365": connection}})
}
