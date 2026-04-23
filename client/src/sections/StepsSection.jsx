const StepsSection = () => (
  <section className="section" id="how">
    <div className="container">
      <div className="section-head">
        <div>
          <div className="section-kicker">Как это работает</div>
          <h2 className="section-title">Три шага до готовой калькуляции.</h2>
        </div>
        <p className="section-lede">
          Без внедрения, без обучения команды, без таблиц. Войдите через Яндекс ID и начните
          считать уже в первый день.
        </p>
      </div>
      <div className="steps">
        <article className="step">
          <div className="step-num">01</div>
          <h3 className="step-title">Настройте правила</h3>
          <p className="step-desc">
            Один раз задайте свои изделия, материалы, операции и наценки. Дальше они подставляются сами.
          </p>
          <div className="step-visual">
            <div className="mini">
              <div className="mini-row"><span>Пиджак</span><span className="v">12 опер.</span></div>
              <div className="mini-row"><span>Блуза</span><span className="v">6 опер.</span></div>
              <div className="mini-row"><span>Брюки</span><span className="v">8 опер.</span></div>
              <div className="mini-bar" />
            </div>
          </div>
        </article>
        <article className="step">
          <div className="step-num">02</div>
          <h3 className="step-title">Введите параметры заказа</h3>
          <p className="step-desc">
            Изделие, ткань, тираж, срочность, сложные элементы. Не больше полутора минут на заказ.
          </p>
          <div className="step-visual">
            <div className="mini">
              <div className="mini-row"><span>Тираж</span><span className="v">15 шт.</span></div>
              <div className="mini-row"><span>Срочность</span><span className="v">14 дн.</span></div>
              <div className="mini-bar" />
            </div>
          </div>
        </article>
        <article className="step">
          <div className="step-num">03</div>
          <h3 className="step-title">Получите итог и AI-оценку</h3>
          <p className="step-desc">
            Себестоимость, рекомендуемая цена и комментарий по сегменту рынка — всё на одном экране.
          </p>
          <div className="step-visual">
            <div className="mini">
              <div className="mini-row"><span>Себестоимость</span><span className="v">287 450 ₽</span></div>
              <div className="mini-row"><span>Реком. цена</span><span className="v">388 060 ₽</span></div>
              <div className="mini-row"><span>Сегмент</span><span className="v">Средний</span></div>
            </div>
          </div>
        </article>
      </div>
    </div>
  </section>
);

export default StepsSection;
