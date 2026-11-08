package auth

import "errors"

var ErrLoginTaken = errors.New("логин уже занят")
var ErrInvalidInput = errors.New("некорректные данные регистрации")

var ErrUserNotFound = errors.New("пользователь не найден")
var ErrInvalidCredentials = errors.New("неверный логин или пароль")
