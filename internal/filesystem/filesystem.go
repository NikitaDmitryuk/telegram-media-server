package filesystem

import (
	"os"
	"path/filepath"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
)

// OSFileSystem реализует интерфейс файловой системы через стандартную ОС
type OSFileSystem struct{}

// NewOSFileSystem создает новую реализацию файловой системы
func NewOSFileSystem() domain.FileSystemInterface {
	return &OSFileSystem{}
}

// Exists проверяет существование файла или директории
func (*OSFileSystem) Exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// CreateDir создает директорию
func (*OSFileSystem) CreateDir(path string) error {
	const dirPerm = 0o755
	err := os.MkdirAll(path, dirPerm)
	if err != nil {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"create_dir_failed",
			"failed to create directory",
		).WithDetails(map[string]any{
			"path": path,
		})
	}
	return nil
}

// RemoveFile удаляет файл
func (*OSFileSystem) RemoveFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"remove_file_failed",
			"failed to remove file",
		).WithDetails(map[string]any{
			"path": path,
		})
	}
	return nil
}

// RemoveDir удаляет директорию
func (*OSFileSystem) RemoveDir(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"remove_dir_failed",
			"failed to remove directory",
		).WithDetails(map[string]any{
			"path": path,
		})
	}
	return nil
}

// WriteFile записывает данные в файл
func (*OSFileSystem) WriteFile(path string, data []byte) error {
	const filePerm = 0o600 // Более безопасные права доступа
	err := os.WriteFile(path, data, filePerm)
	if err != nil {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"write_file_failed",
			"failed to write file",
		).WithDetails(map[string]any{
			"path": path,
			"size": len(data),
		})
	}
	return nil
}

// ReadFile читает данные из файла
func (*OSFileSystem) ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"read_file_failed",
			"failed to read file",
		).WithDetails(map[string]any{
			"path": path,
		})
	}
	return data, nil
}

// ListFiles возвращает список файлов в директории
func (*OSFileSystem) ListFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"list_files_failed",
			"failed to list files",
		).WithDetails(map[string]any{
			"dir": dir,
		})
	}

	return files, nil
}

// GetFileSize возвращает размер файла
func (*OSFileSystem) GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"get_file_size_failed",
			"failed to get file size",
		).WithDetails(map[string]any{
			"path": path,
		})
	}
	return info.Size(), nil
}

// GetFileModTime возвращает время модификации файла
func (*OSFileSystem) GetFileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, errors.WrapDomainError(
			err,
			errors.ErrorTypeFileSystem,
			"get_file_modtime_failed",
			"failed to get file modification time",
		).WithDetails(map[string]any{
			"path": path,
		})
	}
	return info.ModTime(), nil
}

// MockFileSystem для тестирования
type MockFileSystem struct {
	files         map[string][]byte
	dirs          map[string]bool
	shouldError   bool
	errorToReturn error
}

// NewMockFileSystem создает mock файловую систему
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

// SetError заставляет файловую систему возвращать ошибку
func (m *MockFileSystem) SetError(err error) {
	m.shouldError = true
	m.errorToReturn = err
}

// AddFile добавляет файл в mock файловую систему
func (m *MockFileSystem) AddFile(path string, data []byte) {
	m.files[path] = data
}

// AddDir добавляет директорию в mock файловую систему
func (m *MockFileSystem) AddDir(path string) {
	m.dirs[path] = true
}

// Exists mock реализация
func (m *MockFileSystem) Exists(path string) bool {
	if m.shouldError {
		return false
	}
	_, fileExists := m.files[path]
	_, dirExists := m.dirs[path]
	return fileExists || dirExists
}

// CreateDir mock реализация
func (m *MockFileSystem) CreateDir(path string) error {
	if m.shouldError {
		return m.errorToReturn
	}
	m.dirs[path] = true
	return nil
}

// RemoveFile mock реализация
func (m *MockFileSystem) RemoveFile(path string) error {
	if m.shouldError {
		return m.errorToReturn
	}
	delete(m.files, path)
	return nil
}

// RemoveDir mock реализация
func (m *MockFileSystem) RemoveDir(path string) error {
	if m.shouldError {
		return m.errorToReturn
	}
	delete(m.dirs, path)
	// Удаляем все файлы в директории
	for filePath := range m.files {
		if filepath.Dir(filePath) == path {
			delete(m.files, filePath)
		}
	}
	return nil
}

// WriteFile mock реализация
func (m *MockFileSystem) WriteFile(path string, data []byte) error {
	if m.shouldError {
		return m.errorToReturn
	}
	m.files[path] = data
	return nil
}

// ReadFile mock реализация
func (m *MockFileSystem) ReadFile(path string) ([]byte, error) {
	if m.shouldError {
		return nil, m.errorToReturn
	}
	data, exists := m.files[path]
	if !exists {
		return nil, errors.NewDomainError(
			errors.ErrorTypeFileSystem,
			"file_not_found",
			"file not found",
		)
	}
	return data, nil
}

// ListFiles mock реализация
func (m *MockFileSystem) ListFiles(dir string) ([]string, error) {
	if m.shouldError {
		return nil, m.errorToReturn
	}

	var files []string
	for filePath := range m.files {
		if filepath.Dir(filePath) == dir {
			files = append(files, filePath)
		}
	}
	return files, nil
}

// GetFileSize mock реализация
func (m *MockFileSystem) GetFileSize(path string) (int64, error) {
	if m.shouldError {
		return 0, m.errorToReturn
	}
	data, exists := m.files[path]
	if !exists {
		return 0, errors.NewDomainError(
			errors.ErrorTypeFileSystem,
			"file_not_found",
			"file not found",
		)
	}
	return int64(len(data)), nil
}

// GetFileModTime mock реализация
func (m *MockFileSystem) GetFileModTime(path string) (time.Time, error) {
	if m.shouldError {
		return time.Time{}, m.errorToReturn
	}
	if _, exists := m.files[path]; !exists {
		return time.Time{}, errors.NewDomainError(
			errors.ErrorTypeFileSystem,
			"file_not_found",
			"file not found",
		)
	}
	return time.Now(), nil
}
