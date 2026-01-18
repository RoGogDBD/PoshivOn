import { useCallback, useEffect, useState } from "react";
import "./App.css";
import Header from "./components/Header.jsx";
import Footer from "./components/Footer.jsx";
import AuthModal from "./components/AuthModal.jsx";
import HeroSection from "./sections/HeroSection.jsx";
import FeaturesSection from "./sections/FeaturesSection.jsx";
import SolutionsSection from "./sections/SolutionsSection.jsx";
import CasesSection from "./sections/CasesSection.jsx";
import CtaSection from "./sections/CtaSection.jsx";
import { cases, features, footerContacts, navItems, solutions } from "./data/landing.js";
import { useAuthModal } from "./hooks/useAuthModal.js";
import AuthCallback from "./pages/AuthCallback.jsx";
import Panel from "./pages/Panel.jsx";
import { checkAuthStatus } from "./utils/yandexAuth.js";

function App() {
  const [isAuthOpen, setIsAuthOpen] = useState(false);
  const handleAuthClose = useCallback(() => setIsAuthOpen(false), []);
  const handleAuthOpen = useCallback(() => setIsAuthOpen(true), []);

  useAuthModal(isAuthOpen, handleAuthClose);

  useEffect(() => {
    const pathname = window.location.pathname;
    if (pathname.startsWith("/auth") || pathname.startsWith("/panel")) {
      return;
    }

    checkAuthStatus()
      .then((isAuthed) => {
        if (isAuthed) {
          window.location.replace("/panel");
        }
      })
      .catch(() => {});
  }, []);

  if (window.location.pathname.startsWith("/auth")) {
    return <AuthCallback />;
  }
  if (window.location.pathname.startsWith("/panel")) {
    return <Panel />;
  }

  return (
    <div className="page">
      <Header navItems={navItems} onAuthOpen={handleAuthOpen} />

      <main>
        <HeroSection />
        <FeaturesSection items={features} />
        <SolutionsSection items={solutions} />
        <CasesSection items={cases} />
        <CtaSection />
      </main>

      <Footer navItems={navItems} contacts={footerContacts} />
      <AuthModal isOpen={isAuthOpen} onClose={handleAuthClose} />
    </div>
  );
}

export default App;
