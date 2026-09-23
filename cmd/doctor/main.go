package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/doctor"
)

var version = "dev"

func main() {
	publicURL := flag.String("url", os.Getenv("CODEBRIDGE_PUBLIC_URL"), "public CodeBridge origin, for example https://codebridge.example.com")
	adminURL := flag.String("admin-url", "", "optional private/loopback Manager origin for admin checks")
	adminToken := flag.String("admin-token", os.Getenv("CODEBRIDGE_ADMIN_TOKEN"), "admin bearer token; prefer environment variable")
	accessToken := strings.TrimSpace(os.Getenv("CODEBRIDGE_ACCESS_TOKEN"))
	deviceID := flag.String("device-id", "", "optional enrolled device ID to validate")
	workspace := flag.String("workspace", "", "optional logical workspace for a live project_info MCP round-trip; requires --device-id")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	noOAuth := flag.Bool("no-oauth", false, "do not require OAuth protected-resource metadata/challenge")
	noAdminBlock := flag.Bool("no-admin-block-check", false, "skip checking that public /admin is blocked")
	asJSON := flag.Bool("json", false, "print machine-readable JSON")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	results, ok := doctor.Run(context.Background(), doctor.Options{
		PublicURL:        strings.TrimSpace(*publicURL),
		AdminURL:         strings.TrimSpace(*adminURL),
		AdminToken:       strings.TrimSpace(*adminToken),
		AccessToken:      accessToken,
		DeviceID:         strings.TrimSpace(*deviceID),
		Workspace:        strings.TrimSpace(*workspace),
		Timeout:          *timeout,
		ExpectOAuth:      !*noOAuth,
		ExpectAdminBlock: !*noAdminBlock,
	})

	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"ok":      ok,
			"results": results,
		})
	} else {
		for _, result := range results {
			status := "PASS"
			if !result.OK {
				status = "FAIL"
			}
			if result.Detail == "" {
				fmt.Printf("[%s] %s\n", status, result.Name)
			} else {
				fmt.Printf("[%s] %-22s %s\n", status, result.Name, result.Detail)
			}
		}
	}
	if !ok {
		os.Exit(1)
	}
}
