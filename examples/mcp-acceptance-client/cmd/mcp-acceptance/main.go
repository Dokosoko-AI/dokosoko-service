package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dokosoko/mcp-acceptance-client/internal/acceptance"
)

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	code, err := execute(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(code)
}

func execute(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "run" {
		if len(args) > 0 {
			args = args[1:]
		}
		return runSuite(args, stdout, stderr)
	}
	if args[0] == "oauth" {
		if len(args) < 2 {
			return 2, errors.New("usage: mcp-acceptance oauth <login|start|finish>")
		}
		switch args[1] {
		case "login":
			return oauthLogin(args[2:], stdout, stderr)
		case "start":
			return oauthStart(args[2:], stdout, stderr)
		case "finish":
			return oauthFinish(args[2:], stdout, stderr)
		default:
			return 2, errors.New("usage: mcp-acceptance oauth <login|start|finish>")
		}
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		writeUsage(stdout)
		return 0, nil
	}
	return 2, fmt.Errorf("unknown command %q", args[0])
}

func runSuite(args []string, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", "", "Stateless MCPv2 endpoint URL")
	var allowedLoopbackHTTP stringList
	flags.Var(&allowedLoopbackHTTP, "allow-loopback-http", "exact loopback host:port allowed to use plain HTTP for local development; repeatable")
	origin := flags.String("origin", "", "optional browser Origin header")
	tokenFile := flags.String("token-file", "", "0600 file containing a token or OAuth token JSON")
	tokenEnv := flags.String("token-env", "MCP_ACCESS_TOKEN", "environment variable containing the primary token")
	restrictedTokenFile := flags.String("restricted-token-file", "", "0600 file containing a restricted token")
	restrictedTokenEnv := flags.String("restricted-token-env", "MCP_RESTRICTED_ACCESS_TOKEN", "environment variable containing a restricted token")
	var expectedTools stringList
	var expectedResources stringList
	flags.Var(&expectedTools, "expect-tool", "tool name expected in tools/list; repeatable")
	flags.Var(&expectedResources, "expect-resource", "resource URI expected in resources/list; repeatable")
	callTool := flags.String("call-tool", "", "tool to invoke for a positive tools/call check")
	callArgsFile := flags.String("call-args-file", "", "JSON object containing positive tool arguments")
	callConfirmed := flags.Bool("call-confirmed", false, "mark the positive tool call confirmed")
	grantTool := flags.String("grant-tool", "", "tool visible to the primary token and hidden from the restricted token")
	grantArgsFile := flags.String("grant-args-file", "", "JSON object for the optional restricted invocation")
	verifyRestrictedCall := flags.Bool("verify-restricted-call-denied", false, "also invoke the grant tool with the restricted token and require denial")
	confirmationTool := flags.String("confirmation-tool", "", "tool that must reject an unconfirmed invocation")
	confirmationArgsFile := flags.String("confirmation-args-file", "", "JSON object for confirmation checks")
	verifyConfirmedCall := flags.Bool("verify-confirmed-call", false, "also execute the confirmation tool with confirmed=true")
	checkUnauthenticated := flags.Bool("check-unauthenticated", false, "require a request without a token to receive HTTP 401 or 403")
	timeout := flags.Duration("timeout", 20*time.Second, "per-request timeout")
	format := flags.String("format", "human", "report format: human or json")
	reportFile := flags.String("report-file", "", "optional report output file")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *endpoint == "" {
		return 2, errors.New("--endpoint is required")
	}
	token, err := acceptance.LoadToken(*tokenFile, *tokenEnv)
	if err != nil {
		return 2, fmt.Errorf("load primary token: %w", err)
	}
	restrictedToken, err := acceptance.LoadToken(*restrictedTokenFile, *restrictedTokenEnv)
	if err != nil {
		return 2, fmt.Errorf("load restricted token: %w", err)
	}
	callArguments, err := readArguments(*callArgsFile)
	if err != nil {
		return 2, fmt.Errorf("call arguments: %w", err)
	}
	grantArguments, err := readArguments(*grantArgsFile)
	if err != nil {
		return 2, fmt.Errorf("grant arguments: %w", err)
	}
	confirmationArguments, err := readArguments(*confirmationArgsFile)
	if err != nil {
		return 2, fmt.Errorf("confirmation arguments: %w", err)
	}
	report, err := acceptance.Run(context.Background(), acceptance.Config{
		Endpoint: *endpoint, AllowedLoopbackHTTP: allowedLoopbackHTTP,
		Origin: *origin, Token: token, RestrictedToken: restrictedToken,
		ExpectedTools: expectedTools, ExpectedResources: expectedResources,
		CallTool: *callTool, CallArguments: callArguments, CallConfirmed: *callConfirmed,
		GrantTool: *grantTool, GrantArguments: grantArguments, VerifyRestrictedCallDenied: *verifyRestrictedCall,
		ConfirmationTool: *confirmationTool, ConfirmationArguments: confirmationArguments, VerifyConfirmedCall: *verifyConfirmedCall,
		CheckUnauthenticated: *checkUnauthenticated, Timeout: *timeout,
	})
	if err != nil {
		return 2, err
	}
	var writer io.Writer = stdout
	var file *os.File
	if *reportFile != "" {
		file, err = os.OpenFile(*reportFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return 2, err
		}
		defer file.Close()
		writer = file
	}
	switch *format {
	case "human":
		err = acceptance.WriteHuman(writer, report)
	case "json":
		err = acceptance.WriteJSON(writer, report)
	default:
		return 2, errors.New("--format must be human or json")
	}
	if err != nil {
		return 2, err
	}
	return exitCodeForReport(report), nil
}

func exitCodeForReport(report acceptance.Report) int {
	if !report.Accepted() {
		return 1
	}
	return 0
}

func oauthLogin(args []string, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("oauth login", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", "", "private MCP resource endpoint")
	var allowedLoopbackHTTP stringList
	flags.Var(&allowedLoopbackHTTP, "allow-loopback-http", "exact loopback host:port allowed to use plain HTTP for local development; repeatable")
	issuer := flags.String("issuer", "", "OAuth issuer; defaults to endpoint origin")
	metadataURL := flags.String("metadata-url", "", "authorization server metadata URL override")
	resource := flags.String("resource", "", "RFC 8707 resource; defaults to endpoint")
	listenAddress := flags.String("listen", "127.0.0.1:0", "loopback callback listen address; port 0 chooses a free port")
	callbackPath := flags.String("callback-path", "/callback", "exact loopback callback path")
	scope := flags.String("scope", "mcp:private", "OAuth scope")
	clientName := flags.String("client-name", "MCP acceptance client", "dynamic client name")
	clientID := flags.String("client-id", "", "existing public client id; skips DCR")
	stateFile := flags.String("state-file", ".mcp-oauth-state.json", "new 0600 resume-state file")
	tokenFile := flags.String("token-file", "mcp-token.json", "new 0600 token output file")
	timeout := flags.Duration("timeout", 5*time.Minute, "maximum time to wait for the browser callback")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *endpoint == "" {
		return 2, errors.New("--endpoint is required")
	}
	result, err := acceptance.LoginOAuth(context.Background(), acceptance.OAuthLoginConfig{
		Start: acceptance.OAuthStartConfig{
			Issuer: *issuer, MetadataURL: *metadataURL, Endpoint: *endpoint, Resource: *resource,
			AllowedLoopbackHTTP: allowedLoopbackHTTP,
			Scope:               *scope, ClientName: *clientName, ClientID: *clientID,
			StateFile: *stateFile, TokenFile: *tokenFile,
		},
		ListenAddress: *listenAddress,
		CallbackPath:  *callbackPath,
		Timeout:       *timeout,
		OnAuthorizationURL: func(authorizationURL string) {
			fmt.Fprintln(stdout, "Open this authorization URL in a browser:")
			fmt.Fprintln(stdout, authorizationURL)
			fmt.Fprintf(stdout, "\nWaiting up to %s for the loopback callback. This command does not open a browser.\n", *timeout)
		},
	})
	if err != nil {
		return 2, err
	}
	fmt.Fprintf(stdout, "OAuth succeeded. The bearer token was stored with mode 0600 at %s and was not printed.\n", result.TokenFile)
	return 0, nil
}

func oauthStart(args []string, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("oauth start", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", "", "private MCP resource endpoint")
	var allowedLoopbackHTTP stringList
	flags.Var(&allowedLoopbackHTTP, "allow-loopback-http", "exact loopback host:port allowed to use plain HTTP for local development; repeatable")
	issuer := flags.String("issuer", "", "OAuth issuer; defaults to endpoint origin")
	metadataURL := flags.String("metadata-url", "", "authorization server metadata URL override")
	resource := flags.String("resource", "", "RFC 8707 resource; defaults to endpoint")
	redirectURI := flags.String("redirect-uri", "", "registered HTTPS or loopback HTTP callback")
	scope := flags.String("scope", "mcp:private", "OAuth scope")
	clientName := flags.String("client-name", "MCP acceptance client", "dynamic client name")
	clientID := flags.String("client-id", "", "existing public client id; skips DCR")
	stateFile := flags.String("state-file", ".mcp-oauth-state.json", "new 0600 resume-state file")
	tokenFile := flags.String("token-file", "mcp-token.json", "suggested 0600 token output path")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	if *endpoint == "" || *redirectURI == "" {
		return 2, errors.New("--endpoint and --redirect-uri are required")
	}
	result, err := acceptance.StartOAuth(context.Background(), acceptance.OAuthStartConfig{
		Issuer: *issuer, MetadataURL: *metadataURL, Endpoint: *endpoint, Resource: *resource,
		AllowedLoopbackHTTP: allowedLoopbackHTTP,
		RedirectURI:         *redirectURI, Scope: *scope, ClientName: *clientName, ClientID: *clientID,
		StateFile: *stateFile, TokenFile: *tokenFile,
	})
	if err != nil {
		return 2, err
	}
	fmt.Fprintln(stdout, "Open this authorization URL in a browser:")
	fmt.Fprintln(stdout, result.AuthorizationURL)
	fmt.Fprintln(stdout, "\nAfter authorization, resume with:")
	fmt.Fprintln(stdout, result.ResumeCommand)
	fmt.Fprintf(stdout, "\nResume state was stored with mode 0600 at %s. It contains no access token.\n", result.StateFile)
	return 0, nil
}

func oauthFinish(args []string, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("oauth finish", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateFile := flags.String("state-file", ".mcp-oauth-state.json", "0600 resume-state file")
	tokenFile := flags.String("token-file", "mcp-token.json", "new 0600 token output file")
	callbackURL := flags.String("callback-url", "", "full callback URL containing code and state")
	code := flags.String("code", "", "authorization code when callback URL is unavailable")
	returnedState := flags.String("returned-state", "", "returned OAuth state with --code")
	if err := flags.Parse(args); err != nil {
		return 2, err
	}
	result, err := acceptance.FinishOAuth(context.Background(), acceptance.OAuthFinishConfig{
		StateFile: *stateFile, TokenFile: *tokenFile, CallbackURL: *callbackURL,
		Code: *code, ReturnedState: *returnedState,
	})
	if err != nil {
		return 2, err
	}
	fmt.Fprintf(stdout, "OAuth succeeded. The bearer token was stored with mode 0600 at %s and was not printed.\n", result.TokenFile)
	return 0, nil
}

func readArguments(path string) (map[string]any, error) {
	if path == "" {
		return map[string]any{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(value) > 1<<20 {
		return nil, errors.New("argument file exceeds 1 MiB")
	}
	var result map[string]any
	if json.Unmarshal(value, &result) != nil || result == nil {
		return nil, errors.New("argument file must contain one JSON object")
	}
	return result, nil
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  mcp-acceptance run --endpoint URL [options]
  mcp-acceptance oauth login --endpoint URL [options]
  mcp-acceptance oauth start --endpoint URL --redirect-uri URL [options]
  mcp-acceptance oauth finish --state-file FILE --callback-url URL --token-file FILE

Run "mcp-acceptance run -h" or an OAuth subcommand with -h for all flags.`)
}
