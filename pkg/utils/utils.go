package utils

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// GetFilesInDirectory is a convenience function to DRY out some of the
// basic file operations. It accepts a path, resolves it to an absolute path,
// and then uses `os.ReadDir` to retreive and return a slice of `fs.DirEntry` structs.
func GetFilesInDirectory(path string) ([]fs.DirEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve absolute path for '%s': %v", path, err)
	}

	files, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read files in directory '%s': %v", absPath, err)
	}

	return files, nil
}

// FileLine contains fields corresponding to a single line entry
// as received by `bufio.Scanner`. If Err is set, that means that a
// non EOF error was received, indicating a file read failure of some
// kind. So Err should be checked for `!= nil` prior to trying to use
// the Text of the line.
type FileLine struct {
	Text string
	Err  error
}

// ReadFileLines accepts a path, and returns a channel of type FileLine.
// It also spawns an anonymous goroutine that opens the file with a
// line-based scanner (`bufio.Scannner`) to scan each line in the file
// and immediately send it to the channel for the consumer. Because the
// channel is unbuffered, consumers will block while waiting.
func ReadFileLines(path string) chan FileLine {
	lines := make(chan FileLine)

	go func() {
		defer close(lines)
		absPath, err := filepath.Abs(path)
		if err != nil {
			lines <- FileLine{Err: fmt.Errorf("Failed to retrieve absolute path for '%s': %v", path, err)}
			return
		}

		file, err := os.Open(absPath)
		if err != nil {
			lines <- FileLine{Err: fmt.Errorf("Failed to open file '%s': %v", path, err)}
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)

		for scanner.Scan() {
			lines <- FileLine{Text: scanner.Text()}
		}

		if err := scanner.Err(); err != nil {
			lines <- FileLine{Err: scanner.Err()}
		}
	}()

	return lines
}

// GetHostname is a wrapper around `os.Hostname()`. Since the hostname is how
// Mango determines what configurations are applicable to the running system,
// the hostname is critical. It returns the hostname as a string if successful,
// and exits fatally if it fails.
func GetHostname() string {
	h, err := os.Hostname()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to get hostname! Mango cannot determine the system's identity and is unable to determine what configurations are applicable.")
	}

	return h
}
