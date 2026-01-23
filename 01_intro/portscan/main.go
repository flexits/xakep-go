package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type ScanResult struct {
	Host   string
	Port   string
	Banner string
}

// grabBanner читает ответ сервера.
//
// conn  Соединение; должно быть открыто.
// Установите предварительно таймаут чтения и записи.
//
// buf   Временный буфер для хранения ответа.
// Может быть переиспользован между вызовами функции,
// но не конкурентно.
//
// Возвращает ответ и флаг успешности чтения
// (false, если чтение не удалось или ответ был пустым).
func grabBanner(conn net.Conn, buf []byte) (string, bool) {
	request := fmt.Sprintf("HEAD / HTTP/1.1\r\nHost: %s\r\n\r\n", conn.RemoteAddr())
	conn.Write([]byte(request))
	n, err := conn.Read(buf)
	if err == nil && n > 0 {
		return string(buf[:n]), true
	}
	return "", false
}

// loadPortsList загружает список портов из файла.
// Формат исходного файла - одно численное значение на строку.
func loadPortsList(fileName string) ([]string, error) {
	ports := []string{}

	prtFile, err := os.Open(fileName)
	if err != nil {
		return ports, err
	}
	defer prtFile.Close()

	scanner := bufio.NewScanner(prtFile)
	for scanner.Scan() {
		s := scanner.Text()
		_, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			continue
		}
		ports = append(ports, s)
	}
	err = scanner.Err()
	return ports, err
}

// Основную работу выполняем здесь;
// обрати внимание на именованное возвращаемое значение,
// используемое в defer.
func run() (err error) {
	const (
		srcFileName = "targets.txt"
		prtFileName = "ports.txt"
		dstFileName = "scan_results.txt"
	)

	// загружаем список портов из файла
	ports, err := loadPortsList(prtFileName)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		e := fmt.Sprintf("error reading %s: empty file\n", prtFileName)
		return errors.New(e)
	}

	// открываем список целевых хостов и создаём Scanner
	srcFile, err := os.Open(srcFileName)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	scanner := bufio.NewScanner(srcFile)

	// создаём файл для записи результатов и Writer
	dstFile, err := os.Create(dstFileName)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	writer := bufio.NewWriter(dstFile)
	defer func() {
		// запланируем сброс буфера и объединим ошибки
		flushErr := writer.Flush()
		err = errors.Join(err, flushErr)
	}()
	writer.WriteString("[")
	defer writer.WriteString("\n]")

	buffer := make([]byte, 256)
	timeout := 250 * time.Millisecond
	isFirslElem := true
	result := &ScanResult{}
	// каждая строка - отдельный хост;
	// валидируем и сканируем
	for scanner.Scan() {
		target := scanner.Text()
		// валидация хоста
		if net.ParseIP(target) == nil {
			ips, err := net.LookupHost(target)
			if err != nil || len(ips) == 0 {
				continue
			}
			target = ips[0]
		}
		// запускаем скан
		fmt.Printf("Scanning %s ...\n", target)
		for _, port := range ports {
			addr := net.JoinHostPort(target, port)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				continue
			}
			// пишем в консоль
			s := fmt.Sprintf("%s is open\n", addr)
			fmt.Print(s)
			// соберём баннер
			deadline := time.Now().Add(timeout)
			conn.SetDeadline(deadline)
			banner, _ := grabBanner(conn, buffer)
			fmt.Println(banner)
			// соединение больше не нужно
			conn.Close()
			// пишем в файл
			result.Host = target
			result.Port = port
			result.Banner = banner
			jsonBytes, err := json.Marshal(result)
			//jsonBytes, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				continue
			}
			if isFirslElem {
				writer.WriteByte(',')
				isFirslElem = false
			}
			writer.WriteByte('\n')
			writer.Write(jsonBytes)
		}
	}
	// проверим ошибку чтения файла с целями
	return scanner.Err()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
