package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type SubdomainResult struct {
	Name string
	Code int
}

func run() error {
	const srcFileName = "subdomains.txt"

	// целевой хост - аргумент запуска
	if len(os.Args) <= 1 {
		fmt.Fprintf(os.Stderr, "Target address not specified\n")
		os.Exit(1)
	}
	host := os.Args[1]

	// открываем список поддоменов
	srcFile, err := os.Open(srcFileName)
	if err != nil {
		return fmt.Errorf("opening %s: %w", srcFileName, err)
	}
	defer srcFile.Close()

	// настраиваем HTTP клиент
	client := &http.Client{
		Timeout: 1 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// игнорируем редиректы
			return http.ErrUseLastResponse
		},
	}

	// хранилище результатов
	results := []SubdomainResult{}

	var mu sync.Mutex

	var wg sync.WaitGroup

	// построчно считываем и проверяем поддомены
	scanner := bufio.NewScanner(srcFile)
	for scanner.Scan() {
		sub := strings.TrimSpace(scanner.Text())
		if sub == "" {
			continue
		}

		wg.Go(func() {
			target := "https://" + sub + "." + host
			resp, err := client.Get(target)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode == http.StatusNotFound {
				return
			}
			fmt.Printf("*")
			result := SubdomainResult{
				Name: target,
				Code: resp.StatusCode,
			}
			mu.Lock()
			defer mu.Unlock()
			results = append(results, result)
		})
	}
	wg.Wait()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", srcFileName, err)
	}

	fmt.Println()
	for _, r := range results {
		fmt.Printf("%s - %d %s\n", r.Name, r.Code, http.StatusText(r.Code))
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
