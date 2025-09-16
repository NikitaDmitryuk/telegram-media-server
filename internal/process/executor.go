package process

import (
	"context"
	"os"
	"os/exec"
	"strconv"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// OSProcessExecutor выполняет процессы через стандартную ОС
type OSProcessExecutor struct{}

// NewOSProcessExecutor создает новый исполнитель процессов
func NewOSProcessExecutor() domain.ProcessExecutor {
	return &OSProcessExecutor{}
}

// Execute выполняет команду и возвращает результат
func (*OSProcessExecutor) Execute(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	logger.Log.WithFields(map[string]any{
		"command": command,
		"args":    args,
	}).Debug("Executing command")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, errors.WrapDomainError(
			err,
			errors.ErrorTypeExternal,
			"command_execution_failed",
			"failed to execute command",
		).WithDetails(map[string]any{
			"command": command,
			"args":    args,
			"output":  string(output),
		})
	}

	return output, nil
}

// ExecuteWithProgress выполняет команду с отслеживанием прогресса
func (*OSProcessExecutor) ExecuteWithProgress(
	ctx context.Context,
	command string,
	args []string,
	_ chan<- int,
) error {
	cmd := exec.CommandContext(ctx, command, args...)

	logger.Log.WithFields(map[string]any{
		"command": command,
		"args":    args,
	}).Debug("Executing command with progress tracking")

	// Здесь должна быть логика парсинга вывода команды для извлечения прогресса
	// Это зависит от конкретной команды (yt-dlp, aria2c и т.д.)

	err := cmd.Run()
	if err != nil {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeExternal,
			"command_execution_failed",
			"failed to execute command with progress",
		).WithDetails(map[string]any{
			"command": command,
			"args":    args,
		})
	}

	return nil
}

// Kill завершает процесс по PID
func (*OSProcessExecutor) Kill(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeExternal,
			"process_not_found",
			"failed to find process",
		).WithDetails(map[string]any{
			"pid": pid,
		})
	}

	err = process.Kill()
	if err != nil {
		return errors.WrapDomainError(
			err,
			errors.ErrorTypeExternal,
			"process_kill_failed",
			"failed to kill process",
		).WithDetails(map[string]any{
			"pid": pid,
		})
	}

	return nil
}

// MockProcessExecutor для тестирования
type MockProcessExecutor struct {
	commands      []CommandCall
	outputs       map[string][]byte
	errors        map[string]error
	shouldError   bool
	errorToReturn error
}

// CommandCall представляет вызов команды
type CommandCall struct {
	Command string
	Args    []string
}

// NewMockProcessExecutor создает mock исполнитель процессов
func NewMockProcessExecutor() *MockProcessExecutor {
	return &MockProcessExecutor{
		commands: make([]CommandCall, 0),
		outputs:  make(map[string][]byte),
		errors:   make(map[string]error),
	}
}

// SetOutput устанавливает вывод для команды
func (m *MockProcessExecutor) SetOutput(command string, output []byte) {
	m.outputs[command] = output
}

// SetError устанавливает ошибку для команды
func (m *MockProcessExecutor) SetCommandError(command string, err error) {
	m.errors[command] = err
}

// SetGlobalError заставляет исполнитель всегда возвращать ошибку
func (m *MockProcessExecutor) SetGlobalError(err error) {
	m.shouldError = true
	m.errorToReturn = err
}

// GetCommands возвращает список выполненных команд
func (m *MockProcessExecutor) GetCommands() []CommandCall {
	return m.commands
}

// Execute mock реализация
func (m *MockProcessExecutor) Execute(_ context.Context, command string, args ...string) ([]byte, error) {
	m.commands = append(m.commands, CommandCall{
		Command: command,
		Args:    args,
	})

	if m.shouldError {
		return nil, m.errorToReturn
	}

	if err, exists := m.errors[command]; exists {
		return nil, err
	}

	if output, exists := m.outputs[command]; exists {
		return output, nil
	}

	return []byte("mock output"), nil
}

// ExecuteWithProgress mock реализация
func (m *MockProcessExecutor) ExecuteWithProgress(
	ctx context.Context,
	command string,
	args []string,
	progressChan chan<- int,
) error {
	m.commands = append(m.commands, CommandCall{
		Command: command,
		Args:    args,
	})

	if m.shouldError {
		return m.errorToReturn
	}

	if err, exists := m.errors[command]; exists {
		return err
	}

	// Симулируем прогресс
	if progressChan != nil {
		go func() {
			defer close(progressChan)
			for i := 0; i <= 100; i += 25 {
				select {
				case progressChan <- i:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return nil
}

// Kill mock реализация
func (m *MockProcessExecutor) Kill(pid int) error {
	if m.shouldError {
		return m.errorToReturn
	}

	pidStr := strconv.Itoa(pid)
	if err, exists := m.errors["kill_"+pidStr]; exists {
		return err
	}

	return nil
}
