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

type Result struct {
	Name string
	Code int
}

type PipelineConfig struct {
	JobCh       chan string
	ResultCh    chan Result
	ErrCh       chan error
	CliHandle   *http.Client
	SrcFileName string
	DstFileName string
	HostName    string
}

// produce генерирует задания для обработки,
// комбинируя HostName со значениями,
// считанными из SrcFileName,
// и помещает их в канал JobCh.
// Возникающие ошибки отправляются в ErrCh.
func produce(cfg *PipelineConfig) {
	file, err := os.Open(cfg.SrcFileName)
	if err != nil {
		cfg.ErrCh <- fmt.Errorf("opening %s: %w", cfg.SrcFileName, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if s == "" {
			continue
		}
		cfg.JobCh <- "https://" + cfg.HostName + "/" + s
	}

	if err := scanner.Err(); err != nil {
		cfg.ErrCh <- fmt.Errorf("reading %s: %w", cfg.SrcFileName, err)
	}
}

// worker получает значения из канала JobCh, пока он остаётся открытым,
// выполняет обработку и помещает результаты в ResultCh.
func worker(cfg *PipelineConfig) {
	for job := range cfg.JobCh {
		resp, err := cfg.CliHandle.Get(job)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		/*if resp.StatusCode == http.StatusNotFound {
			continue
		}*/
		result := Result{
			Name: job,
			Code: resp.StatusCode,
		}
		cfg.ResultCh <- result
	}
}

// collect получает значения из канала ResultCh, пока он остаётся открытым,
// и записывает их в файл DstFileName.
// Возникающие ошибки отправляются в канал ErrCh.
func collect(cfg *PipelineConfig) {
	dstFile, err := os.Create(cfg.DstFileName)
	if err != nil {
		cfg.ErrCh <- fmt.Errorf("creating %s: %w", cfg.DstFileName, err)
		return
	}
	defer dstFile.Close()

	writer := bufio.NewWriter(dstFile)

	for r := range cfg.ResultCh {
		s := fmt.Sprintf("%s - %d %s\n", r.Name, r.Code, http.StatusText(r.Code))
		_, err = writer.WriteString(s)
		if err != nil {
			cfg.ErrCh <- fmt.Errorf("writing to %s: %w", cfg.DstFileName, err)
		}
	}

	if err = writer.Flush(); err != nil {
		cfg.ErrCh <- fmt.Errorf("writing to %s: %w", cfg.DstFileName, err)
	}
}

func main() {
	const (
		srcFileName = "fuzz.txt"
		dstFileName = "results.txt"
		maxWorkers  = 20
	)

	// целевой хост - аргумент запуска
	if len(os.Args) <= 1 {
		fmt.Fprintf(os.Stderr, "Target address not specified\n")
		os.Exit(1)
	}
	host := os.Args[1]

	// настроенный экземпляр HTTP клиента
	client := &http.Client{
		Timeout: 1 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// игнорируем редиректы
			return http.ErrUseLastResponse
		},
	}

	// канал с заданиями
	jobCh := make(chan string, maxWorkers)
	// канал с результатами
	resultCh := make(chan Result, maxWorkers)
	// канал с ошибками
	errCh := make(chan error, 3)

	config := &PipelineConfig{
		JobCh:       jobCh,
		ResultCh:    resultCh,
		ErrCh:       errCh,
		CliHandle:   client,
		HostName:    host,
		SrcFileName: srcFileName,
		DstFileName: dstFileName,
	}

	// группа конвейера обработки
	var pipelineWg sync.WaitGroup
	// группа пула воркеров
	var workerWg sync.WaitGroup

	// запускаем конвейер обработки
	pipelineWg.Go(func() {
		collect(config)
	})
	pipelineWg.Go(func() {
		produce(config)
		close(jobCh)
	})
	for range maxWorkers {
		workerWg.Go(func() {
			worker(config)
		})
	}

	// закрываем канал результатов по завершению пула воркеров
	go func() {
		workerWg.Wait()
		close(resultCh)
	}()

	// закрываем канал ошибок по завершению конвейера
	go func() {
		pipelineWg.Wait()
		close(errCh)
	}()

	// сбор ошибок
	hasError := false
	for err := range errCh {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error %v\n", err)
			hasError = true
		}
	}
	if hasError {
		os.Exit(1)
	}
}
