# IKEv2 Tunnel Manager

[![CI](https://github.com/vailcody/IKEv2TunnelManager/actions/workflows/ci.yml/badge.svg)](https://github.com/vailcody/IKEv2TunnelManager/actions/workflows/ci.yml)
[![Release](https://github.com/vailcody/IKEv2TunnelManager/actions/workflows/release.yml/badge.svg)](https://github.com/vailcody/IKEv2TunnelManager/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Кроссплатформенное GUI приложение для настройки и управления двухуровневым IKEv2 туннелем.

## 📸 Скриншоты

![Connection](docs/home.png)
*Вкладка Connection — настройка серверов*

![Status](docs/status.png)
*Вкладка Status — мониторинг туннеля*

![Users](docs/users.png)
*Вкладка Users — управление пользователями*

## 📥 Скачать

Перейдите на [страницу Releases](https://github.com/vailcody/IKEv2TunnelManager/releases/latest) и скачайте версию для вашей операционной системы:

| Платформа | Архитектура | Файл |
|-----------|-------------|------|
| 🐧 Linux | x64 | `tunnelmanager-linux-amd64.tar.gz` |
| 🍎 macOS | Intel | `tunnelmanager-macos-amd64.tar.gz` |
| 🍎 macOS | Apple Silicon | `tunnelmanager-macos-arm64.tar.gz` |
| 🪟 Windows | x64 | `tunnelmanager-windows-amd64.zip` |

## 🏗 Архитектура

```
[User] → [Server 1: Tunnel Server + Client] → [Server 2: Exit Node] → [Internet]
```

Пользователь подключается к Server 1, трафик проходит через Server 2, и получает IP-адрес Server 2.

## 📋 Требования

- **Для запуска:** скачайте готовый бинарник из [Releases](https://github.com/vailcody/IKEv2TunnelManager/releases/latest)
- **Для сборки:** Go 1.21+; на Windows дополнительно — MSYS2 и GCC (см. раздел «Из исходников»)
- **Целевые серверы:** Ubuntu 20.04+, root-доступ, порты 500/udp, 4500/udp

## 🚀 Установка

### Из релизов (рекомендуется)

1. Скачайте архив для вашей ОС со [страницы Releases](https://github.com/vailcody/IKEv2TunnelManager/releases/latest)
2. Распакуйте:
   ```bash
   # Linux/macOS
   tar -xzvf tunnelmanager-*.tar.gz
   chmod +x tunnelmanager-*
   ./tunnelmanager-*

   # Windows - распакуйте ZIP и запустите .exe
   ```

### Из исходников

**Linux / macOS:**

```bash
# Клонирование
git clone https://github.com/vailcody/IKEv2TunnelManager
cd IKEv2TunnelManager

# Зависимости (Linux)
sudo apt-get install libgl1-mesa-dev xorg-dev

# Сборка и запуск
go build -o tunnelmanager ./cmd/vpnmanager
./tunnelmanager
```

**Windows:**

Приложение на Fyne требует CGO и компилятор GCC. Рекомендуемая последовательность:

1. **Установите Go** — [golang.org/dl](https://go.dev/dl/)
2. **Установите MSYS2** — содержит компилятор GCC:
   ```powershell
   winget install MSYS2.MSYS2
   ```
   После установки закройте и откройте терминал.
3. **Установите GCC в MSYS2:**
   - Запустите **MSYS2 UCRT64** из меню Пуск
   - Выполните: `pacman -S --noconfirm mingw-w64-ucrt-x86_64-gcc`
4. **Соберите приложение:**
   ```powershell
   cd IKEv2TunnelManager
   powershell -ExecutionPolicy Bypass -File .\build.ps1
   ```
   Первая сборка может занять 1–2 минуты.
5. **Запуск:** `.\tunnelmanager.exe`

**Ручная сборка** (если скрипт недоступен из‑за политики выполнения):
```powershell
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH
go env -w CGO_ENABLED=1
go build -ldflags="-H windowsgui -s -w" -o tunnelmanager.exe ./cmd/vpnmanager
```
> Флаг `-H windowsgui` убирает консольное окно при запуске GUI.

## 📖 Использование

### Вкладка Connection
1. Введите параметры SSH для обоих серверов:
   - **Host**: IP-адрес или hostname
   - **User**: пользователь SSH (обычно root)
   - **Password** или **SSH Key**: способ аутентификации
2. Нажмите **Test Connections** для проверки подключений
3. Нажмите **Setup IKEv2 Tunnel** для полной настройки

### Вкладка Status
- Просмотр текущего состояния туннеля
- Количество активных клиентов
- Статус туннеля между серверами
- Кнопки для перезапуска туннеля

### Вкладка Users
- Добавление/удаление пользователей туннеля
- Список существующих пользователей

### Вкладка Logs
- Журнал операций приложения
- Получение логов StrongSwan с серверов

## 📱 Подключение клиентов

После настройки используйте следующие параметры для подключения:
- **Server**: IP-адрес Server 1
- **Type**: IKEv2
- **Authentication**: Username/Password
- **Username/Password**: созданные во вкладке Users

### iOS/macOS
Settings → VPN → Add VPN Configuration → IKEv2 (IKEv2 Tunnel)

### Windows
Settings → Network → VPN → Add a VPN connection → IKEv2

### Android
Используйте приложение strongSwan VPN Client

## 🔧 Разработка

### Кросс-компиляция

```bash
# Linux (без CGO)
GOOS=linux GOARCH=amd64 go build -o tunnelmanager-linux ./cmd/vpnmanager

# macOS
GOOS=darwin GOARCH=amd64 go build -o tunnelmanager-mac ./cmd/vpnmanager
```
> **Windows:** сборка требует CGO и MinGW. Кросс-компиляция с Linux/macOS сложна — используйте сборку на самой Windows (см. раздел «Из исходников»).

### Запуск тестов
```bash
go test -v ./...
```

## 🤝 Contributing
Мы приветствуем вклад в проект! Смотрите [CONTRIBUTING.md](CONTRIBUTING.md) для деталей.

## 📄 License
MIT License - смотрите [LICENSE](LICENSE) для деталей.
