#!/bin/sh
# entrypoint db-service: MariaDB и Go-обёртка (dbservice) — один инстанс Serverless
# Container, оба процесса запускаются здесь (см. план миграции, раздел «Фаза 2»).
#
# Диск инстанса эфемерный (принятый риск R1 плана) — это значит, что datadir MariaDB
# ПУСТ при каждом холодном старте, не только при первом деплое. Официальный entrypoint
# образа mariadb сам обнаруживает пустой datadir и делает полную инициализацию
# (mariadb-install-db, создание БД/пользователя из MARIADB_DATABASE/MARIADB_USER/
# MARIADB_PASSWORD) при каждом запуске — этим объясняется задержка холодного старта
# ~10-15с, заложенная в execution-timeout ревизии (см. Фазу 2 плана).
set -e

# В среде выполнения Serverless Containers /tmp оказался недоступен на запись для
# процесса mariadbd (Errcode 13, обнаружено на реальном деплое) — InnoDB не может создать
# временные файлы там и падает уже на этапе инициализации, до старта самого сервера.
# /var/lib/mysql (datadir) писать умеет — это подтверждено тем же логом, инициализация
# самого datadir там успевает пройти раньше, чем падает InnoDB. TMPDIR — переменная
# окружения, которую и mariadb-install-db, и сам mariadbd используют как источник
# tmpdir по умолчанию, если явно не переопределено иначе, — задаём её один раз здесь,
# и она подхватывается обоими процессами официального entrypoint'а.
mkdir -p /var/lib/mysql/tmp
chown mysql:mysql /var/lib/mysql/tmp 2>/dev/null || true
export TMPDIR=/var/lib/mysql/tmp

docker-entrypoint.sh mariadbd &
mariadb_pid=$!

# Готовность проверяется под прикладным пользователем (DB_USER/DB_PASSWORD) — теми же
# credentials, которыми будет подключаться сам dbservice ниже, а не под root: если вход
# под прикладным пользователем уже проходит, инициализация БД/пользователя официальным
# entrypoint'ом гарантированно завершена.
#
# Цикл ограничен по попыткам и проверяет живость процесса mariadbd на каждой итерации:
# без этого сломанная инициализация (плохой конфиг, повреждённый образ) вешала бы
# контейнер молча до истечения внешнего execution-timeout платформы, без единой строки
# в логе о реальной причине.
attempt=0
# 90 попыток × 0.5с = 45с — с запасом под execution-timeout ревизии (обнаружено на
# практике: на 20%-доле vCPU инициализация MariaDB не укладывалась в прежние 30с).
# MAX_READY_ATTEMPTS — настраивается через окружение, не требует пересборки образа, если
# понадобится подвинуть снова.
max_attempts="${MAX_READY_ATTEMPTS:-90}"
until mariadb-admin ping -h "${DB_HOST:-127.0.0.1}" -P "${DB_PORT:-3306}" \
    -u"${DB_USER}" -p"${DB_PASSWORD}" --silent 2>/dev/null; do
  if ! kill -0 "$mariadb_pid" 2>/dev/null; then
    echo "docker-entrypoint-dbservice: mariadbd (pid $mariadb_pid) завершился во время инициализации" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  if [ "$attempt" -ge "$max_attempts" ]; then
    echo "docker-entrypoint-dbservice: MariaDB не ответила за $((max_attempts / 2)) секунд" >&2
    exit 1
  fi
  sleep 0.5
done

# Сам Go-процесс запускается от непривилегированного пользователя (см. Dockerfile.dbservice)
# — ему не нужен root, в отличие от mariadbd, который уронил привилегии до mysql сам,
# через официальный entrypoint, ещё до этой строки.
exec gosu dbservice-app dbservice
