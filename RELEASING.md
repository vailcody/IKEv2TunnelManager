# Releasing

Инструкции по публикации новых версий.

## Создание релиза

1. Убедитесь, что все изменения закоммичены и протестированы
2. Создайте тег с версией:

```bash
# Для финальных релизов
git tag -a v1.0.0 -m "Release v1.0.0"

# Для бета-версий
git tag -a v1.0.0-beta.1 -m "Beta release v1.0.0-beta.1"
```

3. Отправьте тег на GitHub:

```bash
git push origin v1.0.0
```

4. GitHub Actions автоматически:
   - Соберёт приложение для всех платформ (Linux, macOS, Windows)
   - Создаст страницу релиза
   - Загрузит архивы для скачивания

## Версионирование

Используем [Semantic Versioning](https://semver.org/):

- `MAJOR.MINOR.PATCH` (например, `v1.2.3`)
- `MAJOR` — несовместимые изменения API
- `MINOR` — новая функциональность, совместимая с предыдущими версиями
- `PATCH` — исправления багов

### Пре-релизы

- `v1.0.0-alpha.1` — альфа-версии
- `v1.0.0-beta.1` — бета-версии  
- `v1.0.0-rc.1` — релиз-кандидаты

Пре-релизы будут помечены соответственно на странице релизов.

## Проверка перед релизом

```bash
# Запуск тестов
go test -v ./...

# Проверка кода
go vet ./...

# Сборка
go build -v ./cmd/vpnmanager
```

## Структура релиза

После создания тега, на странице релизов появятся:

| Файл | Платформа |
|------|-----------|
| `vpnmanager-linux-amd64.tar.gz` | Linux x64 |
| `vpnmanager-linux-arm64.tar.gz` | Linux ARM64 |
| `vpnmanager-macos-amd64.tar.gz` | macOS Intel |
| `vpnmanager-macos-arm64.tar.gz` | macOS Apple Silicon |
| `vpnmanager-windows-amd64.zip` | Windows x64 |
