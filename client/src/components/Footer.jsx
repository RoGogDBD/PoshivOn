const Footer = ({ contacts }) => (
  <footer className="footer">
    <div className="container">
      <div className="footer-grid">
        <div className="footer-col footer-brand">
          <div className="brand">
            Poshiv<span className="brand-dot">On</span>
          </div>
          <p>Сервис для расчёта стоимости пошива и управления заказами.</p>
        </div>
        <div className="footer-col">
          <h4>Продукт</h4>
          <a href="#how">Как это работает</a>
          <a href="#capabilities">Возможности</a>
          <a href="#example">Пример расчёта</a>
        </div>
        <div className="footer-col">
          <h4>Компания</h4>
          <a href="#audience">Кому подходит</a>
          <a href="#faq">Вопросы</a>
          {contacts.map((item) =>
            item.href ? (
              <a key={item.href} href={item.href}>{item.label}</a>
            ) : (
              <span key={item.label}>{item.label}</span>
            )
          )}
        </div>
      </div>
      <div className="footer-bottom">
        <span>© 2026 PoshivOn</span>
      </div>
    </div>
  </footer>
);

export default Footer;
