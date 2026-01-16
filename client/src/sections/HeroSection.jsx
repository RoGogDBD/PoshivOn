const HeroSection = () => (
  <section className="hero">
    <div className="container hero__inner">
      <div className="hero__content">
        <p className="eyebrow">Посчитайте стоимость пошива</p>
        <h1>
          Сервис <span>PoshivOn</span> за <span>2 минуты</span> расчитает точную себестоимость - без ошибок, споров и пересчетов.
        </h1>
        <p className="subtitle">
          Автоматизируйте расчеты в PoshivOn: точная калькуляция ткани,
          фурнитуры и труда — всё в одном сервисе.
        </p>
      </div>
      <div className="hero__visual">
        <img className="hero__image" src="/sewing_machine.svg" alt="Швейная машина" />
      </div>
    </div>
  </section>
);

export default HeroSection;
