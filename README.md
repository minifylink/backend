# backend

go/psql endpoints
```
// https::/.../add
post
request = {
  url: "URL" // string
}
answer = {
  state: STATE // state_enum
  state_msg: "MSG" // string
  short_url_if_possible: "URL" // string
}

// https::/.../stat
post
request = {
  url: "URL" // string
}
answer = {
  state: STATE // state_enum
  state_msg: "MSG" // string
  stat_url_if_possible: 000 // unsigned int
}

enum state_enum = {
  SUCCESS
  ERROR
}

## Запуск с использованием Docker

Для запуска сервиса с использованием Docker и Docker Compose выполните следующие шаги:

1. Клонируйте репозиторий:
```bash
git clone https://github.com/yourusername/backend.git
cd backend
```

2. Создайте файл .env на основе примера:
```bash
cp .env.example .env
```

3. Отредактируйте .env файл, установив необходимые пароли и настройки

4. При необходимости адаптируйте скрипт инициализации базы данных в `docker/init-db.sh`

5. Запустите сервисы с помощью Docker Compose:
```bash
docker-compose up -d
```

6. Сервис будет доступен по адресу http://localhost:8082

### База данных

При первом запуске PostgreSQL автоматически инициализируется скриптом `docker/init-db.sh`, который создает необходимые таблицы:
- `links` - для хранения ссылок
- `users` - для хранения пользователей
- `analytics` - для хранения аналитики по переходам

Настройки подключения к базе данных берутся из файла `config/prod.yaml` и переменных окружения в `.env`.

### Отладка соединения с базой данных

Если у вас возникают проблемы с подключением к базе данных, вы можете использовать следующие команды для отладки:

1. Запуск тестового сервиса для проверки соединения с базой данных:
```bash
docker-compose --profile debug up db-test
```

2. Проверка логов PostgreSQL:
```bash
docker-compose logs postgres
```

3. Проверка логов бэкенд-сервиса:
```bash
docker-compose logs backend
```

4. Вход в контейнер PostgreSQL для ручной проверки:
```bash
docker-compose exec postgres psql -U postgres -d shortener
```

5. Если драйвер "pgx" не найден, убедитесь, что в файле repository.go используется строка `sql.Open("pgx", ...)` вместо `sql.Open("postgres", ...)`, а также правильно импортирован пакет: `_ "github.com/jackc/pgx/v5/stdlib"`.

### Сборка Docker образа вручную

Если вы хотите собрать Docker образ вручную:

```bash
docker build -t url-shortener .
```

Запуск контейнера:

```bash
docker run -p 8082:8082 -e HTTP_SERVER_PASSWORD=your_password url-shortener
```
