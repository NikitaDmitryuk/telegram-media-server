#!/bin/bash

# Проверяем, существует ли файл messages.yaml
if [ ! -f messages.yaml ]; then
    echo "Ошибка: файл messages.yaml не найден!"
    exit 1
fi

# Указываем суффикс для имен констант
SUFFIX="MsgID"

# Инициализируем файл messages_gen.go
echo "package main" > messages_gen.go
echo "" >> messages_gen.go
echo "// Этот файл сгенерирован автоматически из messages.yaml" >> messages_gen.go
echo "" >> messages_gen.go
echo "type MessageID string" >> messages_gen.go
echo "" >> messages_gen.go
echo "const (" >> messages_gen.go

# Используем awk для разбора файла YAML и извлечения ключей верхнего уровня
awk -v suffix="$SUFFIX" '
BEGIN { FS=":"; OFS=""; inside=0 }
{
    if ($0 ~ /^[^ \t]/ && $0 !~ /^#/) {
        # Ключ верхнего уровня
        gsub(":", "", $1)
        key = $1
        # Заменяем дефисы и пробелы на подчеркивания для корректных идентификаторов Go
        gsub("[- ]", "_", key)
        # Добавляем суффикс к имени константы
        const_name = key suffix
        print "    ", const_name, " MessageID = \"" $1 "\""
    }
}
' messages.yaml >> messages_gen.go

echo ")" >> messages_gen.go

echo "Файл messages_gen.go успешно сгенерирован."
