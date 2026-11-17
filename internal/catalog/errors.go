package catalog

import "errors"

var ErrInvalidInput = errors.New("некорректные параметры списка файлов")
var ErrFileNotFound = errors.New("файл не найден")
