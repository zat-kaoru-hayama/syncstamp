package main

import (
	"bytes"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type File struct {
	os.FileInfo
	Path string
	hash []byte
}

var openCount int = 0

func hash(path string) ([]byte, error) {
	h := md5.New()

	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	io.Copy(h, fd)
	openCount++

	return h.Sum(nil)[:], nil
}

func (f *File) Hash() ([]byte, error) {
	if f.hash == nil {
		var err error
		f.hash, err = hash(f.Path)
		if err != nil {
			return nil, err
		}
	}
	return f.hash, nil
}

func (this *File) Equal(other *File) (bool, error) {
	hash1, err := this.Hash()
	if err != nil {
		return false, err
	}
	hash2, err := other.Hash()
	if err != nil {
		return false, err
	}
	return bytes.Equal(hash1, hash2), nil
}

func (this *File) Sametime(other *File) bool {
	time1 := this.ModTime().Truncate(time.Second)
	time2 := other.ModTime().Truncate(time.Second)
	return time1.Equal(time2)
}

func findSameFileButTimeDiff(srcFiles []*File, dstFile *File) (*File, error) {
	for _, srcFile := range srcFiles {
		if srcFile.Sametime(dstFile) {
			continue
		}
		equal, err := srcFile.Equal(dstFile)
		if err != nil {
			return nil, err
		}
		if equal {
			return srcFile, nil
		}
	}
	return nil, nil
}

var flagBatch = flag.Bool("batch", false, "output batchfile to stdout")

var flagUpdate = flag.Bool("update", false, "update destinate-file's timestamp same as source-file's one")

type keyT struct {
	Name string
	Size int64
}

func walk(root string, callback func(*keyT, *File) error) error {
	if path, err := filepath.EvalSymlinks(root); err == nil {
		root = path
	}
	return filepath.Walk(root, func(path string, file1 os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if file1.IsDir() {
			if name := filepath.Base(path); name[0] == '.' {
				return filepath.SkipDir
			}
			return nil
		}
		key := &keyT{
			Name: strings.ToUpper(filepath.Base(path)),
			Size: file1.Size(),
		}
		val := &File{Path: path, FileInfo: file1}
		return callback(key, val)
	})
}

func getTree(root string) (map[keyT][]*File, int, error) {
	files := map[keyT][]*File{}
	count := 0

	err := walk(root, func(key *keyT, value *File) error {
		files[*key] = append(files[*key], value)
		count++
		return nil
	})
	return files, count, err
}

func mains(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("Usage: %s <SRC-DIR> <DST-DIR>", os.Args[0])
	}

	srcRoot := args[0]

	source, srcCount, err := getTree(srcRoot)
	if err != nil {
		return err
	}

	dstRoot := args[1]
	dstCount := 0
	updCount := 0

	err = walk(dstRoot, func(key *keyT, val *File) error {
		dstCount++

		srcFiles, ok := source[*key]
		if !ok {
			return nil
		}

		matchSrcFile, err := findSameFileButTimeDiff(
			srcFiles,
			val)
		if err != nil {
			return err
		}
		if matchSrcFile == nil {
			return nil
		}
		if *flagBatch {
			fmt.Printf("touch -r \"%s\" \"%s\"\n",
				matchSrcFile.Path,
				val.Path)
		} else {
			fmt.Printf("   %s %s\n",
				matchSrcFile.ModTime().Format("2006/01/02 15:04:05"), matchSrcFile.Path)
			if *flagUpdate {
				fmt.Print("->")
			} else {
				fmt.Print("!=")
			}

			fmt.Printf(" %s %s\n\n",
				val.ModTime().Format("2006/01/02 15:04:05"), val.Path)

			if *flagUpdate {
				os.Chtimes(val.Path,
					matchSrcFile.ModTime(),
					matchSrcFile.ModTime())
				updCount++
			}
		}
		return nil
	})
	fmt.Fprintf(os.Stderr, "    Read %4d files on %s.\n", srcCount, srcRoot)
	fmt.Fprintf(os.Stderr, "Compared %4d files on %s.\n", dstCount, dstRoot)
	if updCount > 0 {
		fmt.Fprintf(os.Stderr, " Touched %4d files on %s.\n", updCount, dstRoot)
	}
	fmt.Fprintf(os.Stderr, "    Open %4d files.\n", openCount)
	return err
}

func main() {
	flag.Parse()
	if err := mains(flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
