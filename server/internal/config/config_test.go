package config

import "testing"

// TestLoad_ContactEmail: CONTACT_EMAIL действительно читается из окружения.
//
// Тест, проверяющий только «значение из Config доезжает до ответа», прошёл бы и на поле,
// которое Load() никогда не заполняет: тогда на проде плашка показывала бы пустой контакт
// при заданной переменной. Здесь проверяется вторая половина пути — от окружения до Config.
func TestLoad_ContactEmail(t *testing.T) {
	// Load() ищет .env вверх по дереву и подставляет из него незаданные переменные. Пустой
	// временный каталог делает тест независимым от того, что лежит в .env у разработчика:
	// иначе строка CONTACT_EMAIL в локальном файле ломала бы ветку «по умолчанию пусто».
	t.Run("значение берётся из окружения", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("CONTACT_EMAIL", "help@poshivon.example")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.ContactEmail != "help@poshivon.example" {
			t.Errorf("ContactEmail = %q, ожидался help@poshivon.example", cfg.ContactEmail)
		}
	})

	t.Run("по умолчанию пусто", func(t *testing.T) {
		// Переменная не задана — плашка обходится без контакта, а не падает.
		t.Chdir(t.TempDir())
		t.Setenv("CONTACT_EMAIL", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.ContactEmail != "" {
			t.Errorf("ContactEmail = %q, ожидалась пустая строка", cfg.ContactEmail)
		}
	})
}
