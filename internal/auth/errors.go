package auth

import "errors"

var ErrLoginTaken = errors.New("логин уже занят")
var ErrInvalidInput = errors.New("некорректные данные регистрации")
