pkgname=bbg-telegram-media-server
pkgver=1.0.28
pkgrel=1
pkgdesc="Telegram Media Server"
arch=('aarch64' 'armv7h' 'x86_64')
url="https://github.com/NikitaDmitryuk/bbg-telegram-media-server-golang"
license=('MIT')
makedepends=('go')
depends=('yt-dlp')
source=()

install="${pkgname}.install"
options=(!strip)

build() {
    cd "${srcdir}/../"

    case "$CARCH" in
        'aarch64')
            env GOOS=linux GOARCH=arm64 go build -o "${srcdir}/bbg-telegram-media-server" .
            ;;
        'armv7h')
            env GOOS=linux GOARCH=arm GOARM=7 go build -o "${srcdir}/bbg-telegram-media-server" .
            ;;
        'x86_64')
            env GOOS=linux GOARCH=amd64 go build -o "${srcdir}/bbg-telegram-media-server" .
            ;;
        *)
            echo "Unsupported architecture: $CARCH"
            exit 1
            ;;
    esac
}

package() {
    cd "${srcdir}/../"
    install -Dm755 "${srcdir}/bbg-telegram-media-server" "${pkgdir}/usr/bin/bbg-telegram-media-server"
    install -Dm644 .env.example "${pkgdir}/etc/bbg-telegram-media-server/.env.example"
    install -Dm644 ${pkgname}.service "${pkgdir}/usr/lib/systemd/system/bbg-telegram-media-server.service"
}
