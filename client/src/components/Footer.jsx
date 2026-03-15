const Footer = ({ navItems, contacts }) => (
  <footer className="footer">
    <div className="container footer__inner">
      <div>
        <div className="logo">PoshivOn</div>
        <p>Сервис для расчета стоимости пошива и управления заказами.</p>
      </div>
      <div className="footer__links">
        {navItems.map((item) => (
          <a key={item.href} href={item.href}>
            {item.label}
          </a>
        ))}
      </div>
      <div className="footer__contacts">
        {contacts.map((item) =>
          item.href ? (
            <a key={item.href} href={item.href}>
              {item.label}
            </a>
          ) : (
            <span key={item.label}>{item.label}</span>
          )
        )}
      </div>
    </div>
  </footer>
);

export default Footer;
