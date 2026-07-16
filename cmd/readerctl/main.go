package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/andrewedunn/agent-reading-relay/internal/cli"
)

func main() {
	dependencies := cli.Dependencies{Prompt: terminalPrompt}
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, dependencies); err != nil {
		fmt.Fprintln(os.Stderr, "readerctl:", err)
		os.Exit(1)
	}
}

func terminalPrompt(label string, secret bool) (string, error) {
	if _, err := fmt.Fprintf(os.Stderr, "%s: ", label); err != nil {
		return "", err
	}
	if secret {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return "", fmt.Errorf("secret input requires an interactive terminal")
		}
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(value), err
	}
	value, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(value), err
}
