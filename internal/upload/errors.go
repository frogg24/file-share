package upload

import "errors"

var ErrInvalidInput = errors.New("некорректные данные файла")
var ErrFileTooLarge = errors.New("файл превышает допустимый размер")
var ErrFileNotFound = errors.New("файл не найден")
