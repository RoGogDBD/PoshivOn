import { useEffect, useState } from "react";

const Header = ({ navItems, onAuthOpen }) => {
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  useEffect(() => {
    document.body.classList.toggle("mobile-menu-open", isMenuOpen);
    return () => document.body.classList.remove("mobile-menu-open");
  }, [isMenuOpen]);

  useEffect(() => {
    const mediaQuery = window.matchMedia("(min-width: 900px)");
    const handleViewportChange = (event) => {
      if (event.matches) {
        setIsMenuOpen(false);
      }
    };

    mediaQuery.addEventListener("change", handleViewportChange);
    return () => mediaQuery.removeEventListener("change", handleViewportChange);
  }, []);

  const handleAuthClick = () => {
    setIsMenuOpen(false);
    onAuthOpen();
  };

  return (
    <header className="header">
      <div className="container header__inner">
        <div className="logo">PoshivOn</div>

        <nav className="nav nav--desktop" aria-label="Основная навигация">
          {navItems.map((item) => (
            <a key={item.href} href={item.href}>
              {item.label}
            </a>
          ))}
        </nav>

        <div className="header__actions">
          <button className="btn btn--ghost header__login" type="button" onClick={onAuthOpen}>
            Вход
          </button>
          <button
            className={`nav-toggle ${isMenuOpen ? "nav-toggle--open" : ""}`}
            type="button"
            aria-expanded={isMenuOpen}
            aria-controls="mobile-nav"
            aria-label={isMenuOpen ? "Закрыть меню" : "Открыть меню"}
            onClick={() => setIsMenuOpen((current) => !current)}
          >
            <span />
            <span />
            <span />
          </button>
        </div>
      </div>

      <div
        id="mobile-nav"
        className={`mobile-nav ${isMenuOpen ? "mobile-nav--open" : ""}`}
        aria-hidden={!isMenuOpen}
      >
        <div className="container mobile-nav__inner">
          <nav className="mobile-nav__links" aria-label="Мобильная навигация">
            {navItems.map((item) => (
              <a key={item.href} href={item.href} onClick={() => setIsMenuOpen(false)}>
                {item.label}
              </a>
            ))}
          </nav>
          <button className="btn btn--primary mobile-nav__auth" type="button" onClick={handleAuthClick}>
            Войти в кабинет
          </button>
        </div>
      </div>
    </header>
  );
};

export default Header;
