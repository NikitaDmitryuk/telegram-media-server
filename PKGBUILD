pkgname=telegram-media-server
pkgver=1.1.0
pkgrel=1
pkgdesc="Telegram Media Server"
arch=('aarch64' 'armv7h' 'x86_64')
url="https://github.com/NikitaDmitryuk/telegram-media-server"
license=('MIT')
makedepends=('go')
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
        'aarch64')
            env GOOS=linux GOARCH=arm64 go build -o "${srcdir}/telegram-media-server" ./cmd/telegram-media-server
            ;;
        'armv7h')
            env GOOS=linux GOARCH=arm GOARM=7 go build -o "${srcdir}/telegram-media-server" ./cmd/telegram-media-server
            ;;
        'x86_64')
            env GOOS=linux GOARCH=amd64 go build -o "${srcdir}/telegram-media-server" ./cmd/telegram-media-server
            ;;
        *)
            echo "Unsupported architecture: $CARCH"
            exit 1
            ;;
    esac
}

package() {
    cd "$srcdir"

    # Install the binary
    install -Dm755 "${srcdir}/telegram-media-server" "${pkgdir}/usr/bin/telegram-media-server"

    # Install configuration files
    install -Dm644 .env.example "${pkgdir}/etc/telegram-media-server/.env.example"

    # Install the systemd service file from the build directory
    install -Dm644 "$startdir/build/telegram-media-server.service" "${pkgdir}/usr/lib/systemd/system/telegram-media-server.service"
}
