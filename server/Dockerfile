# Сборка приложения.
ARG GO_VERSION=1.25.5
FROM golang:${GO_VERSION}-alpine AS builder

# Устанавливаем рабочую директорию.
WORKDIR /app

# Копируем файлы модуля.
COPY go.mod go.sum* ./

# Загружаем зависимости (если они есть).
RUN go mod download

# Копируем исходный код.
COPY . .

# Сборка бинарника.
RUN go build -o /app/bin/server ./cmd

# Финальный образ.
FROM alpine:3.20

# Создаем непривилегированного пользователя.
RUN adduser -D -g '' appuser

WORKDIR /app

# Копируем бинарник.
COPY --from=builder /app/bin/server /app/server

USER appuser

# Порт приложения.
EXPOSE 8080

# Запуск приложения.
ENTRYPOINT ["/app/server"]