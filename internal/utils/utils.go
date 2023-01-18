package utils

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

// GetFilesInDirectory is a convenience function to DRY out some of the
// basic file operations. It accepts a path, resolves it to an absolute path,
// and then uses `os.ReadDir` to retreive and return a slice of `fs.DirEntry` structs.
func GetFilesInDirectory(path string) ([]fs.DirEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to retrieve absolute path")

		return nil, err
	}

	files, err := os.ReadDir(absPath)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  absPath,
			"error": err,
		}).Error("Failed to read files in directory")

		return nil, err
	}

	return files, nil
}

// GetFileModifiedTime accepts a path to a file/directory. It returns the
// file's last modified time, or an empty time literal in the event of failure
// (read: if this function fails, it returns the epoch timestamp -- use
// `time.IsZero()` to check if return is the zero time instant for failures).
func GetFileModifiedTime(path string) time.Time {
    file, err := os.Stat(path)
    if err != nil {
	return time.Time{}
    }

    return file.ModTime()
}

// IsFileExecutableToAll accepts an `fs.DirEntry` struct and
// simply returns true if the given file is executable to all
// (ie, `--x--x--x`), else false.
func IsFileExecutableToAll(file fs.DirEntry) bool {
	info, err := file.Info()
	if err != nil {
		log.WithFields(log.Fields{
			"file":  file.Name(),
			"error": err,
		}).Error("Failed to get file info")

		return false
	}

	// check if apply file is executable by all
	if info.Mode()&0111 == 0111 {
		return true
	}

	return false
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
		absPath, err := filepath.Abs(path)
		if err != nil {
			log.WithFields(log.Fields{
				"path":  path,
				"error": err,
			}).Error("Failed to retrieve absolute path")
		}

		file, err := os.Open(absPath)
		if err != nil {
			log.WithFields(log.Fields{
				"path":  absPath,
				"error": err,
			}).Error("Failed to open file")
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)

		for scanner.Scan() {
			lines <- FileLine{Text: scanner.Text()}
		}

		if err := scanner.Err(); err != nil {
			lines <- FileLine{Err: scanner.Err()}
		}

		close(lines)
	}()

	return lines
}
