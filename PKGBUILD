pkgname=bbg_telegram_media_server
pkgver=1.0.0
pkgrel=1
pkgdesc="BBG Telegram Media Server"
arch=('armv7l' 'x86_64')
url="https://github.com/NikitaDmitryuk/bbg-telegram-media-server-golang"
license=('MIT')
depends=('minidlna')
makedepends=('go')
source=()
install="${pkgname}.install"
sha256sums=()

build() {
    cd "${srcdir}/../"
    env GOOS=linux GOARCH=arm go build -o "${srcdir}/bbg_telegram_media_server" .
}

package() {
    cd "${srcdir}/../"
    install -Dm755 "${srcdir}/bbg_telegram_media_server" "${pkgdir}/usr/bin/bbg_telegram_media_server"
    install -Dm644 .env.example "${pkgdir}/etc/bbg_telegram_media_server/.env.example"
    install -Dm644 bbg_telegram_media_server.service "${pkgdir}/usr/lib/systemd/system/bbg_telegram_media_server.service"
}
