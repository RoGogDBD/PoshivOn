import { buildYandexAuthUrl, saveAuthReturnTo } from "../utils/yandexAuth.js";

const AuthModal = ({ isOpen, onClose }) => {
  if (!isOpen) {
    return null;
  }

  const handleAuthClick = () => {
    const authUrl = buildYandexAuthUrl();
    if (!authUrl) {
      console.log("Не задан VITE_YA_CLIENT_ID для Яндекс ID.");
      return;
    }

    const returnTo = `${window.location.pathname}${window.location.search}`;
    saveAuthReturnTo(returnTo);
    window.location.assign(authUrl);
  };

  return (
    <div
      className="modal-overlay"
      role="presentation"
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="auth-title">
        <button className="modal__close" type="button" onClick={onClose} aria-label="Закрыть окно входа">
          ×
        </button>
        <h3 id="auth-title">Вход через Яндекс ID</h3>
        <div className="modal__action">
          <button className="ya-btn" type="button" onClick={handleAuthClick}>
            <span className="ya-btn__icon" aria-hidden="true">
              <span className="ya-btn__icon-letter">Y</span>
            </span>
            Войти с Яндекс ID
          </button>
        </div>
      </div>
    </div>
  );
};

export default AuthModal;
