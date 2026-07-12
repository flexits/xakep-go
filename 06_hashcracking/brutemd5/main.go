package main

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type PipelineConfig struct {
	JobCh               chan []string
	ResultCh            chan string
	ErrorCh             chan<- error
	SrcFileName         string
	TargetHash          *[md5.Size]byte
	Counter             *atomic.Uint64
	CancelCheckInterval int
	BatchSize           int
}

// produce генерирует задания для обработки,
// считывая значения из cfg.SrcFileName,
// и помещает их в канал cfg.JobCh;
// завершается по окончанию чтения файла.
func produce(ctx context.Context, cfg *PipelineConfig) {
	file, err := os.Open(cfg.SrcFileName)
	if err != nil {
		cfg.ErrorCh <- fmt.Errorf("opening %s: %w", cfg.SrcFileName, err)
		return
	}
	defer file.Close()

	// пакет - задание, содержащий строки для обработки
	batch := make([]string, 0, cfg.BatchSize)

	// счётчик считанных строк
	i := 0

	// неблокирующая отправка
	send := func(batch []string) bool {
		select {
		case <-ctx.Done():
			return false
		case cfg.JobCh <- batch:
			return true
		}
	}

	scanner := bufio.NewScanner(file)
	defer func() {
		if err := scanner.Err(); err != nil {
			cfg.ErrorCh <- fmt.Errorf("reading %s: %w", cfg.SrcFileName, err)
		}
	}()
	for scanner.Scan() {
		// периодически проверяем необходимость отмены
		if i%cfg.CancelCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
		i++

		// формируем пакет добавлением строк
		s := strings.TrimSpace(scanner.Text())
		if s == "" {
			continue
		}
		batch = append(batch, s)

		// по достижении заданного размера, отправляем задание
		if len(batch) == cfg.BatchSize {
			if send(batch) {
				batch = make([]string, 0, cfg.BatchSize)
			}
		}
	}

	// отправляем оставшийся неполный пакет
	if len(batch) > 0 {
		send(batch)
	}
}

// worker читает слова из cfg.JobCh,
// вычисляет MD5 хеш и сравнивает его с cfg.TargetHash,
// при совпадении помещает слово в cfg.ResultCh и завершается;
// при отсутствии совпадений завершается
// по закрытию cfg.JobCh или отменой конекста.
func worker(ctx context.Context, cfg *PipelineConfig) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobs, ok := <-cfg.JobCh:
			if !ok {
				return
			}
			for i, job := range jobs {
				// периодически проверяем необходимость отмены
				if i%cfg.CancelCheckInterval == 0 {
					select {
					case <-ctx.Done():
						return
					default:
					}
				}

				cfg.Counter.Add(1)

				if md5.Sum([]byte(job)) == *cfg.TargetHash {
					select {
					case cfg.ResultCh <- job:
					case <-ctx.Done():
					}
					return
				}
			}
		}
	}
}

// collect ожидает чтения либо закрытия cfg.ResultCh,
// выводит значение в консоль и завершается.
func collect(cfg *PipelineConfig) {
	pwd, ok := <-cfg.ResultCh
	if !ok {
		fmt.Println("\rNo match found!")
	} else {
		fmt.Println("\rMatched successfully: ", pwd)
	}
	fmt.Println("Total attempts:", cfg.Counter.Load())
}

func main() {
	const srcFileName = "dict.txt"
	var maxWorkers = runtime.GOMAXPROCS(0)

	// целевой хеш - аргумент запуска
	if len(os.Args) <= 1 {
		fmt.Fprintf(os.Stderr, "Target MD5 hash not specified\n")
		os.Exit(1)
	}
	hashStr := strings.TrimSpace(os.Args[1])
	// валидация и преобразование
	if len(hashStr) != 32 {
		fmt.Fprintf(os.Stderr, "MD5 hash must be 32 characters long\n")
		os.Exit(1)
	}
	var hashBytes [md5.Size]byte
	_, err := hex.Decode(hashBytes[:], []byte(hashStr))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	// канал с заданиями
	jobCh := make(chan []string, maxWorkers)
	// канал с результатами
	resultCh := make(chan string)
	// канал с ошибками
	errCh := make(chan error, 3)
	// счетчик попыток
	var count atomic.Uint64
	// флаг ошибки обработки
	var hadError atomic.Bool

	conf := &PipelineConfig{
		JobCh:               jobCh,
		ResultCh:            resultCh,
		ErrorCh:             errCh,
		SrcFileName:         srcFileName,
		TargetHash:          &hashBytes,
		Counter:             &count,
		CancelCheckInterval: 64,
		BatchSize:           1024,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// профиль CPU
	f, err := os.Create("cpu.prof")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := pprof.StartCPUProfile(f); err != nil {
		panic(err)
	}
	defer pprof.StopCPUProfile()

	// индикатор прогресса
	go func() {
		ticker := time.NewTicker(time.Millisecond * 250)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Printf("\r%d", count.Load())
			case <-ctx.Done():
				return
			}
		}
	}()

	// сбор ошибок
	var errWg sync.WaitGroup
	errWg.Go(func() {
		for err := range errCh {
			if err != nil {
				fmt.Fprintf(os.Stderr, "error %v\n", err)
				hadError.Store(true)
			}
		}
	})

	// запомним время начала работы
	start := time.Now()

	// генерируем задания
	go func() {
		produce(ctx, conf)
		close(jobCh)
	}()

	// запускаем пул обработчиков,
	// по завершению - закрываем каналы
	var workerWg sync.WaitGroup
	for range maxWorkers {
		workerWg.Go(func() {
			worker(ctx, conf)
		})
	}
	go func() {
		workerWg.Wait()
		close(resultCh)
		close(errCh)
	}()

	// останавливаем работу после получения результата
	collect(conf)
	stop()

	// ожидаем завершения сборки ошибок и выбираем код возврата
	errWg.Wait()

	// вычислим длительность работы
	fmt.Printf("Elapsed: %s\n", time.Since(start))

	// профиль памяти
	f, err = os.Create("mem.prof")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
		fmt.Fprintf(os.Stderr, "could not write memory profile: %v\n", err)
	}

	if hadError.Load() {
		os.Exit(1)
	}
}
