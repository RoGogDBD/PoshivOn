import { useState } from "react";

const FAQ_ITEMS = [
  {
    q: "Сколько времени нужно на внедрение?",
    a: "В среднем полчаса. Настраиваете справочник изделий и операций один раз — и сразу начинаете считать заказы. Импорт из Excel поддерживается.",
  },
  {
    q: "Данные заказов и клиентов в безопасности?",
    a: "Да. Авторизация через Яндекс ID, сессии с refresh-механикой, изоляция данных на уровне пользователя. База — MariaDB, возможно on-premise развёртывание.",
  },
  {
    q: "Можно ли задать свои правила ценообразования?",
    a: "Да. Вы сами описываете изделия, операции, материалы, коэффициенты срочности и скидки по объёму. PoshivOn подставляет их во все расчёты автоматически.",
  },
  {
    q: "Чем отличаются режимы quick и masterpiece?",
    a: "Quick — быстрый расчёт по базовым параметрам для коммерческого. Masterpiece — детальная калькуляция стоимости с AI-оценкой сегмента рынка.",
  },
  {
    q: "Что делает AI-оценка рынка?",
    a: "Анализирует параметры заказа через DeepSeek и возвращает комментарий по сегменту, диапазон цены и риски позиционирования. Работа сервиса не требует AI — это дополнительный слой.",
  },
];

const FAQSection = () => {
  const [open, setOpen] = useState(0);
  return (
    <section className="section" id="faq">
      <div className="container faq-grid">
        <div>
          <div className="section-kicker">Вопросы</div>
          <h2 className="section-title">Что обычно спрашивают до внедрения.</h2>
          <p className="section-lede" style={{ marginTop: 20 }}>
            Не нашли ответ — напишите нам, отвечаем в течение рабочего дня.
          </p>
        </div>
        <div className="faq-list">
          {FAQ_ITEMS.map((it, i) => (
            <div className={`faq-item ${open === i ? "open" : ""}`} key={it.q}>
              <button className="faq-q" type="button" onClick={() => setOpen(open === i ? -1 : i)}>
                {it.q}
                <span className="tog">▾</span>
              </button>
              <div className="faq-a">{it.a}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default FAQSection;
