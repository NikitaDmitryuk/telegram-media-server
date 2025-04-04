pkgname=telegram-media-server
pkgver=1.1.6
pkgrel=1
pkgdesc="Telegram Media Server"
arch=('aarch64' 'x86_64')
url="https://github.com/NikitaDmitryuk/telegram-media-server"
license=('MIT')
makedepends=('go' 'aarch64-linux-gnu-gcc')
depends=('yt-dlp' 'aria2')
source=()
install="build/${pkgname}.install"
options=(!strip)

prepare() {
    cd "$srcdir"
    cp -r "$startdir/cmd" .
    cp -r "$startdir/internal" .
    cp "$startdir/go.mod" .
    cp "$startdir/go.sum" .
    cp "$startdir/LICENSE" .
    cp "$startdir/.env.example" .
}

build() {
    cd "$srcdir"
    
    case "$CARCH" in
        aarch64)
            env CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc \
                go build -trimpath -o "$srcdir/telegram-media-server" ./cmd/telegram-media-server
            ;;
        x86_64)
            env CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
                go build -trimpath -o "$srcdir/telegram-media-server" ./cmd/telegram-media-server
            ;;
        *)
            echo "Unsupported architecture: $CARCH"
            exit 1
            ;;
    esac
}

package() {
    cd "$srcdir"
    install -Dm755 "$srcdir/telegram-media-server" "$pkgdir/usr/bin/telegram-media-server"
    install -Dm644 .env.example "$pkgdir/etc/telegram-media-server/.env.example"
    install -Dm644 "$startdir/build/telegram-media-server.service" "$pkgdir/usr/lib/systemd/system/telegram-media-server.service"
}
