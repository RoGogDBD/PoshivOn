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
          <a
            href="https://github.com/RoGogDBD/PoshivOn"
            className="btn btn-outline nav-github"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Репозиторий на GitHub"
          >
            <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18" aria-hidden="true">
              <path d="M12 0C5.374 0 0 5.373 0 12c0 5.302 3.438 9.8 8.207 11.387.6.11.82-.26.82-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0 1 12 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.218.694.825.576C20.565 21.796 24 17.299 24 12c0-6.627-5.373-12-12-12z" />
            </svg>
          </a>
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
