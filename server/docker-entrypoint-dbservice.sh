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

# Периодический дамп в Object Storage (принятая владельцем митигация риска R1 — диск
# инстанса эфемерный, датадир пропадает при каждом пересоздании контейнера, включая
# то, что платформа делает сама по себе даже при min-instances=1). Раз в 6 часов, пока жив
# сам контейнер — при min-instances=0 цикл не имел смысла: инстанс мог не дожить до
# собственного таймера. Фоновый процесс, отдельный от mariadbd и dbservice: ошибка одного
# цикла дампа (сеть, IAM, диск) не должна ронять сам сервис — поэтому всё тело цикла
# написано так, чтобы ни одна команда не завершала фоновый процесс через set -e, только
# логировала и ждала следующего цикла.
dump_and_upload_once() {
  # IAM-токен для сервисного аккаунта инстанса — тот же metadata-эндпоинт и протокол
  # (Metadata-Flavor: Google), что и MetadataTokenSource в Go-коде HTTPRepository; получаем
  # заново на каждый цикл, а не кэшируем — раз в 6 часов это не накладно, а токен живёт
  # меньше суток.
  # "|| token_response=..." обязателен под set -e: без него неудачный curl (сетевой сбой,
  # metadata-сервис недоступен) прервал бы весь фоновый процесс dump_loop молча — команда
  # в присвоении переменной не защищена условием if/while/&&/||, и set -e считает её
  # обычной командой скрипта, а не частью управляемой проверки.
  token_response=$(curl -sS --max-time 10 -H "Metadata-Flavor: Google" \
    "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token" 2>&1) \
    || token_response="curl завершился с ошибкой при обращении к metadata-сервису"
  iam_token=$(echo "$token_response" | grep -o '"access_token"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4)
  if [ -z "$iam_token" ]; then
    echo "db-dump: не удалось получить IAM-токен, пропускаю цикл: $token_response" >&2
    return 1
  fi

  dump_name="poshivon-$(date -u +%Y%m%d-%H%M%S).sql.gz"
  # /var/lib/mysql/tmp — та же директория, что уже создана и chown'нута выше под TMPDIR;
  # единственное подтверждённо писабельное место в этом рантайме (Errcode 13 на /tmp,
  # найдено на реальном деплое) — дамп временно ложится туда же, не в новое место. Лог
  # ошибки — там же, не в /tmp и не в корне образа: только эта директория проверена как
  # писабельная в реальном рантайме.
  dump_sql_path="/var/lib/mysql/tmp/${dump_name%.gz}"
  dump_path="/var/lib/mysql/tmp/${dump_name}"
  dump_err_path="/var/lib/mysql/tmp/dump-err.log"

  # Раздельные команды, не mariadb-dump | gzip одним пайпом: у sh/dash нет pipefail, и
  # if-проверка пайпа смотрела бы только на код возврата gzip — он успешно завершится даже
  # на пустом/оборванном выводе, и настоящий сбой mariadb-dump остался бы незамеченным.
  #
  # MYSQL_PWD, а не -p"$DB_PASSWORD": пароль не должен попадать в argv этого процесса, где
  # его увидел бы любой, кто может прочитать /proc в том же контейнере (то, чего не может
  # быть на существующей проверке готовности mariadb-admin ping чуть выше — она проверяется
  # один раз на старте, до появления какого-либо чужого кода в контейнере; здесь — цикл на
  # весь срок жизни инстанса, лишний риск того не стоит).
  if ! MYSQL_PWD="$DB_PASSWORD" mariadb-dump -h "${DB_HOST:-127.0.0.1}" -P "${DB_PORT:-3306}" \
      -u"${DB_USER}" --single-transaction "${DB_NAME}" >"$dump_sql_path" 2>"$dump_err_path"; then
    echo "db-dump: mariadb-dump завершился с ошибкой: $(cat "$dump_err_path" 2>/dev/null)" >&2
    rm -f "$dump_sql_path" "$dump_err_path"
    return 1
  fi
  rm -f "$dump_err_path"

  if ! gzip "$dump_sql_path"; then
    echo "db-dump: gzip дампа завершился с ошибкой" >&2
    rm -f "$dump_sql_path" "$dump_path"
    return 1
  fi

  if ! curl -sS --max-time 60 --request PUT \
      --header "Authorization: Bearer ${iam_token}" \
      --upload-file "$dump_path" \
      "https://storage.yandexcloud.net/poshivon-db-dumps/${dump_name}" >/dev/null; then
    echo "db-dump: загрузка $dump_name в Object Storage не удалась" >&2
    rm -f "$dump_path"
    return 1
  fi

  rm -f "$dump_path"
  echo "db-dump: $dump_name выгружен успешно"
}

dump_loop() {
  # Первый дамп — сразу после старта, не через 6 часов: инстанс может прожить меньше цикла
  # (редеплой, ручной рестарт), и без этого свежий дамп не появился бы вовсе до первого
  # полного интервала.
  dump_and_upload_once || true
  while true; do
    sleep "${DUMP_INTERVAL_SEC:-21600}"
    dump_and_upload_once || true
  done
}

dump_loop &

# Сам Go-процесс запускается от непривилегированного пользователя (см. Dockerfile.dbservice)
# — ему не нужен root, в отличие от mariadbd, который уронил привилегии до mysql сам,
# через официальный entrypoint, ещё до этой строки.
exec gosu dbservice-app dbservice
