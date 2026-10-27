#!/usr/bin/env bash
# Пусковой файл «Диагностика ПК» для Linux:
# при отсутствии бинарника собирает его, затем запускает программу.
set -e
cd "$(dirname "$0")"

BIN="./pc-diagnostic"

if [ ! -x "$BIN" ]; then
    echo "Бинарник не найден — собираю..."
    if ! command -v go >/dev/null 2>&1; then
        echo "Ошибка: Go не установлен."
        echo "Установите Go 1.21+ (https://go.dev/dl/) или скопируйте собранный бинарник pc-diagnostic."
        exit 1
    fi
    go build -o pc-diagnostic .
fi

exec "$BIN" "$@"
