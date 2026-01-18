const Panel = () => (
  <div className="page panel">
    <aside className="panel__sidebar">
      <div className="panel__brand">PoshivOn</div>
      <nav className="panel__nav">
        <button className="panel__link panel__link--active" type="button">
          История
        </button>
        <button className="panel__link" type="button">
          Настройки
        </button>
      </nav>
    </aside>
    <main className="panel__content">
      <h1>Панель</h1>
      <p>Добро пожаловать! Здесь появится история действий и настройки аккаунта.</p>
    </main>
  </div>
);

export default Panel;
