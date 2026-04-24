import { useEffect, useState } from "react";

const Header = ({ navItems, onAuthOpen }) => {
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  useEffect(() => {
    document.body.classList.toggle("mobile-menu-open", isMenuOpen);
    return () => document.body.classList.remove("mobile-menu-open");
  }, [isMenuOpen]);

  useEffect(() => {
    const mq = window.matchMedia("(min-width: 981px)");
    const handler = (e) => { if (e.matches) setIsMenuOpen(false); };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  const handleAuthClick = () => {
    setIsMenuOpen(false);
    onAuthOpen();
  };

  return (
    <header className="nav-bar">
      <div className="container nav-inner">
        <a href="#" className="brand">
          Poshiv<span className="brand-dot">On</span>
        </a>

        <nav className="nav-links" aria-label="Основная навигация">
          {navItems.map((item) => (
            <a key={item.href} href={item.href}>
              {item.label}
            </a>
          ))}
        </nav>

        <div className="nav-cta">
          <button className="btn btn-ghost" type="button" onClick={onAuthOpen}>
            Войти
          </button>
          <button
            className={`nav-toggle ${isMenuOpen ? "nav-toggle--open" : ""}`}
            type="button"
            aria-expanded={isMenuOpen}
            aria-controls="mobile-nav"
            aria-label={isMenuOpen ? "Закрыть меню" : "Открыть меню"}
            onClick={() => setIsMenuOpen((v) => !v)}
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
        <div className="mobile-nav__inner">
          <nav className="mobile-nav__links" aria-label="Мобильная навигация">
            {navItems.map((item) => (
              <a key={item.href} href={item.href} onClick={() => setIsMenuOpen(false)}>
                {item.label}
              </a>
            ))}
          </nav>
          <button className="btn btn-primary mobile-nav__auth" type="button" onClick={handleAuthClick}>
            Войти в кабинет
          </button>
        </div>
      </div>
    </header>
  );
};

export default Header;
