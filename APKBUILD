pkgname=simplevisor
pkgver=0.3.0
pkgrel=0
pkgdesc="Yet Another Process Supervisor"
url="https://code.crute.us/mcrute/simplevisor/"
arch="all"
license="MIT"
depends=""
makedepends="go"
source="
	https://code.crute.us/mcrute/simplevisor/snapshot/simplevisor-$pkgver.tar.bz2
	"
options="!check net"
#builddir="$srcdir/golib-bin/webp/v$pkgver/bin/webp/"

export GOCACHE="${GOCACHE:-"$srcdir/go-cache"}"
export GOTMPDIR="${GOTMPDIR:-"$srcdir"}"
export GOMODCACHE="${GOMODCACHE:-"$srcdir/go"}"

build() {
	make
}

package() {
	install -Dm755 "$builddir"/$pkgname \
		"$pkgdir"/usr/bin/$pkgname
}

sha512sums="
e9b4bb490b92f6899eedab860ec43e50d38bf704cec168e9395c927b6e88526ff50d4cfa50d49539d2df8e46d30e4e32a470a75ab6b6a3ab1f1d5c627f9576d0  simplevisor-0.3.0.tar.bz2
"
