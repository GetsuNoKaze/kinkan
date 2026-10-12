package presets

// Kinkan: a quiet node by default. A fresh install and a new node get only the
// connections that do not give the node away to a single probe, and the cards say which
// presets do.
func init() {
	for i := range All {
		switch All[i].ID {
		case "tuic_v5":
			// TUIC v5 closes QUIC with its own codes (0xfffffff2 after about a second without
			// credentials, 0xfffffff1 on wrong ones): one probe tells the node apart.
			All[i].Default = false
			All[i].Summary = "Альтернатива на QUIC по UDP. Без ключа закрывает соединение своими кодами, и сканер узнаёт ноду одной пробой: не включайте на ноде, IP которой важно сберечь"
		case "trusttunnel":
			All[i].Summary = "Протокол AdGuard: HTTP/2 на настоящем сертификате ноды. В Kinkan на запрос без ключа отвечает сайтом ноды, как обычный веб-сервер; без сайта ноды отвечает 407 и выдаёт прокси. Только приложения на mihomo"
		}
	}
}
