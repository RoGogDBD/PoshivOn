const Header = ({ navItems, onAuthOpen }) => (
  <header className="header">
    <div className="container header__inner">
      <div className="logo">PoshivOn</div>
      <nav className="nav">
        {navItems.map((item) => (
          <a key={item.href} href={item.href}>
            {item.label}
          </a>
        ))}
      </nav>
      <button className="btn btn--ghost" type="button" onClick={onAuthOpen}>
        Вход
      </button>
    </div>
  </header>
);

export default Header;
