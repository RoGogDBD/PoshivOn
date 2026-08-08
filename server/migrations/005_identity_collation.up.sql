-- Личность пользователя сравнивается побайтово.
--
-- users.id и все ссылающиеся на него user_id наследовали умолчание базы
-- utf8mb4_uca1400_ai_ci — регистронезависимое, акцентонезависимое и игнорирующее концевые
-- пробелы. Проверено живьём на mariadb:11.4: 'RoGogDBD' = 'rogogdbd' = 'RoGogDBD ' и
-- 'e' = 'é'. Логин из Яндекса — это и есть личность (Decision 4: два администратора заданы
-- логинами), поэтому аккаунт, чей логин сворачивался к логину администратора, получал бы
-- его строку users вместе с role='admin' и has_access=1; двое обычных пользователей,
-- сворачивающихся друг к другу, молча делили бы одну строку users со всеми её настройками,
-- чатами и расчётами.
--
-- Выбран utf8mb4_nopad_bin, а не utf8mb4_bin: в MariaDB utf8mb4_bin — это PAD SPACE, и
-- концевые пробелы он всё ещё игнорирует. Проверено живьём на том же сервере:
--   utf8mb4_bin        'RoGogDBD' = 'RoGogDBD ' -> 1  (вектор остаётся открытым)
--   utf8mb4_nopad_bin  'RoGogDBD' = 'RoGogDBD ' -> 0
-- Из трёх доступных двоичных вариантов только NO PAD закрывает все три вектора сразу.
--
-- Данные не меняются — меняются только правила сравнения, поэтому существующие строки
-- остаются прежними побайтово. Появление дубликатов невозможно: под старой, более
-- «склеивающей» сортировкой id был первичным ключом, так что строк, различающихся только
-- регистром, в таблице и не могло быть; ужесточение сравнения способно только разделить
-- то, что раньше склеивалось, но не создать конфликт ключа.
--
-- Порядок обязателен: MariaDB требует совпадения сортировки у обеих сторон внешнего ключа,
-- поэтому сначала снимаются все FK, затем меняются колонки, затем FK возвращаются в
-- прежнем виде. Все операторы идемпотентны (IF EXISTS / IF NOT EXISTS), как в 004.
-- Внимание к форме guard'а: у внешнего ключа IF NOT EXISTS ставится после FOREIGN KEY, а не
-- после CONSTRAINT <имя>, — форма `ADD CONSTRAINT IF NOT EXISTS ... FOREIGN KEY` (та, что
-- работает для CHECK в 004) здесь даёт ERROR 1064.
--
-- calculations.chat_id и chats.id намеренно не трогаются: идентификатор чата генерирует
-- сервер, он не является личностью, а обе стороны fk_calculations_chat остаются с
-- одинаковой сортировкой.

ALTER TABLE user_settings DROP FOREIGN KEY IF EXISTS fk_user_settings_user;
ALTER TABLE chats DROP FOREIGN KEY IF EXISTS fk_chats_user;
ALTER TABLE access_requests DROP FOREIGN KEY IF EXISTS fk_access_requests_user;
ALTER TABLE calculations DROP FOREIGN KEY IF EXISTS fk_calculations_chat;

ALTER TABLE users
  MODIFY COLUMN id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL;

ALTER TABLE user_settings
  MODIFY COLUMN user_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL;

ALTER TABLE chats
  MODIFY COLUMN user_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL;

ALTER TABLE access_requests
  MODIFY COLUMN user_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL;

ALTER TABLE calculations
  MODIFY COLUMN user_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL;

ALTER TABLE user_settings
  ADD CONSTRAINT fk_user_settings_user
    FOREIGN KEY IF NOT EXISTS (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE chats
  ADD CONSTRAINT fk_chats_user
    FOREIGN KEY IF NOT EXISTS (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE access_requests
  ADD CONSTRAINT fk_access_requests_user
    FOREIGN KEY IF NOT EXISTS (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE calculations
  ADD CONSTRAINT fk_calculations_chat
    FOREIGN KEY IF NOT EXISTS (user_id, chat_id) REFERENCES chats(user_id, id) ON DELETE CASCADE;
