package syncengine

import (
	"archive/tar"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

type TarFile struct {
	RelPath string
	Content []byte
	ModTime time.Time
	Mode    int64
	IsDir   bool
}

func WriteTar(dst io.Writer, files []TarFile) error {
	tw := tar.NewWriter(dst)
	for _, file := range files {
		name, err := safeTarPath(file.RelPath)
		if err != nil {
			_ = tw.Close()
			return err
		}
		header := &tar.Header{
			Name:    name,
			Mode:    file.Mode,
			ModTime: file.ModTime,
		}
		if file.IsDir {
			header.Name += "/"
			header.Typeflag = tar.TypeDir
		} else {
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(file.Content))
		}
		if err := tw.WriteHeader(header); err != nil {
			_ = tw.Close()
			return err
		}
		if !file.IsDir {
			if _, err := tw.Write(file.Content); err != nil {
				_ = tw.Close()
				return err
			}
		}
	}
	return tw.Close()
}

func safeTarPath(relPath string) (string, error) {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	if relPath == "" || strings.HasPrefix(relPath, "/") {
		return "", fmt.Errorf("unsafe tar path %q", relPath)
	}
	cleaned := path.Clean(relPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe tar path %q", relPath)
	}
	return cleaned, nil
}
