#!/usr/bin/bash
archs=(arm amd64)

for arch in ${archs[@]}
do
	  env GOOS=linux GOARCH=${arch} go build -o bbg_telegram_media_server_${arch}
done
