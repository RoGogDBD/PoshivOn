import { useCallback, useEffect, useState } from "react";
import "./App.css";
import Header from "./components/Header.jsx";
import Footer from "./components/Footer.jsx";
import AuthModal from "./components/AuthModal.jsx";
import HeroSection from "./sections/HeroSection.jsx";
import FeaturesSection from "./sections/FeaturesSection.jsx";
import StepsSection from "./sections/StepsSection.jsx";
import CasesSection from "./sections/CasesSection.jsx";
import SolutionsSection from "./sections/SolutionsSection.jsx";
import FAQSection from "./sections/FAQSection.jsx";
import CtaSection from "./sections/CtaSection.jsx";
import { footerContacts, navItems } from "./data/landing.js";
import { useAuthModal } from "./hooks/useAuthModal.js";
import AuthCallback from "./pages/AuthCallback.jsx";
import Panel from "./pages/Panel.jsx";
import PrivacyPolicy from "./pages/PrivacyPolicy.jsx";
import Demo from "./pages/Demo.jsx";
import CookieConsent from "./components/CookieConsent.jsx";
import { checkAuthStatus } from "./utils/yandexAuth.js";

function App() {
  const [isAuthOpen, setIsAuthOpen] = useState(false);
  const handleAuthClose = useCallback(() => setIsAuthOpen(false), []);
  const handleAuthOpen = useCallback(() => setIsAuthOpen(true), []);

  useAuthModal(isAuthOpen, handleAuthClose);

  useEffect(() => {
    const pathname = window.location.pathname;
    if (
      pathname.startsWith("/auth") ||
      pathname.startsWith("/panel") ||
      pathname.startsWith("/privacy") ||
      pathname.startsWith("/demo")
    ) {
      return;
    }
    checkAuthStatus()
      .then((isAuthed) => { if (isAuthed) window.location.replace("/panel"); })
      .catch(() => {});
  }, []);

  let content;
  if (window.location.pathname.startsWith("/auth")) {
    content = <AuthCallback />;
  } else if (window.location.pathname.startsWith("/panel")) {
    content = <Panel />;
  } else if (window.location.pathname.startsWith("/privacy")) {
    content = <PrivacyPolicy />;
  } else if (window.location.pathname.startsWith("/demo")) {
    content = <Demo />;
  } else {
    content = (
      <div>
        <Header navItems={navItems} onAuthOpen={handleAuthOpen} />
        <main>
          <HeroSection onAuthOpen={handleAuthOpen} />
          <FeaturesSection />
          <StepsSection />
          <CasesSection />
          <SolutionsSection />
          <FAQSection />
          <CtaSection onAuthOpen={handleAuthOpen} />
        </main>
        <Footer navItems={navItems} contacts={footerContacts} />
        <AuthModal isOpen={isAuthOpen} onClose={handleAuthClose} />
      </div>
    );
  }

  // Уведомление о cookie — на каждом маршруте, где реально идёт обработка Яндекс-логина
  // (она начинается уже на /panel, в т.ч. при заходе туда напрямую по ссылке), кроме /demo:
  // это отдельная витринная страница без входа через Яндекс и без auth-cookie, показывать
  // плашку там нечего обосновывать.
  const isDemo = window.location.pathname.startsWith("/demo");
  return (
    <>
      {content}
      {!isDemo && <CookieConsent />}
    </>
  );
}

export default App;
