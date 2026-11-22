package model

// MinorErrors Канал незначительных ошибок.
// Прерывание контекста игнорируется.
// Не вызывает остановку приложения.
var MinorErrors chan error

// CommonErrors Канал обычных ошибок.
// Прерывание контекста игнорируется.
// Вызывает остановку приложения.
var CommonErrors chan error

// CriticalErrors Канал критических ошибок.
// Ничего не игнорируется.
// Вызывает остановку приложения.
var CriticalErrors chan error
