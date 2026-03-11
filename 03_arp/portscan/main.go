package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"portscan/arp"
	"strconv"
	"strings"
	"time"
)

type ScanResult struct {
	Host   string
	Port   string
	Mac    string
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
		return fmt.Errorf("loading ports: %w", err)
	}
	if len(ports) == 0 {
		return fmt.Errorf("reading %s: empty file", prtFileName)
	}

	// открываем список целевых хостов
	srcFile, err := os.Open(srcFileName)
	if err != nil {
		return fmt.Errorf("opening %s: %w", srcFileName, err)
	}
	defer srcFile.Close()

	// создаём файл для записи результатов
	dstFile, err := os.Create(dstFileName)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dstFileName, err)
	}
	defer dstFile.Close()

	// создаём Scanner и Writer для буферизованного чтения и записи
	scanner := bufio.NewScanner(srcFile)
	writer := bufio.NewWriter(dstFile)
	// пишем первый символ (начало массива JSON)
	_, err = writer.WriteString("[")
	if err != nil {
		return fmt.Errorf("writing to file: %w", err)
	}
	defer func() {
		// запланируем сброс буфера
		flushErr := writer.Flush()
		// объединим ошибки, если есть
		err = errors.Join(err, flushErr)
	}()

	// создаём читателя кеша ARP
	arpReader := arp.NewArpReader()

	buffer := make([]byte, 256)
	timeout := 250 * time.Millisecond
	isFirslElem := true
	result := &ScanResult{}
	// каждая строка - отдельный хост;
	// валидируем и сканируем
	for scanner.Scan() {
		target := strings.TrimSpace(scanner.Text())
		if target == "" {
			continue
		}
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
		// получаем MAC адрес
		mac, found := arpReader.GetMac(target)
		if found {
			fmt.Println(mac)
		}
		for _, port := range ports {
			addr := net.JoinHostPort(target, port)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				// сервер:порт недоступен
				continue
			}
			// сервер:порт доступен, сообщаем в консоль
			fmt.Printf("%s is open\n", addr)
			// соберём баннер
			deadline := time.Now().Add(timeout)
			conn.SetDeadline(deadline)
			banner, _ := grabBanner(conn, buffer)
			fmt.Println(banner)
			// соединение больше не нужно
			conn.Close()
			// обновляем поля объекта-результата
			result.Host = target
			result.Port = port
			result.Mac = mac
			result.Banner = banner
			// и пишем его содержимое в файл
			jsonBytes, err := json.Marshal(result)
			//jsonBytes, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				continue
			}
			if isFirslElem {
				isFirslElem = false
			} else {
				writer.WriteByte(',')
			}
			writer.WriteByte('\n')
			writer.Write(jsonBytes)
		}
	}
	// закрывающий символ массива JSON
	writer.WriteString("\n]")
	// проверим ошибку чтения файла с целями
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
