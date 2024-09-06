pkgname=bbg-telegram-media-server
pkgver=1.0.20
pkgrel=1
pkgdesc="BBG Telegram Media Server"
arch=('aarch64')
url="https://github.com/NikitaDmitryuk/bbg-telegram-media-server-golang"
license=('MIT')
makedepends=('go')
source=()

install="${pkgname}.install"
options=(!strip)

build() {
    cd "${srcdir}/../"
    env GOOS=linux GOARCH=arm64 go build -o "${srcdir}/bbg-telegram-media-server" .
}

package() {
    cd "${srcdir}/../"
    install -Dm755 "${srcdir}/bbg-telegram-media-server" "${pkgdir}/usr/bin/bbg-telegram-media-server"
    install -Dm644 .env.example "${pkgdir}/etc/bbg-telegram-media-server/.env.example"
    install -Dm644 ${pkgname}.service "${pkgdir}/usr/lib/systemd/system/bbg-telegram-media-server.service"
}
