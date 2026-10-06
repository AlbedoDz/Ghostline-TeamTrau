import { useTranslation } from "react-i18next";
import { Service } from "../app/api";
import { useGhost } from "../app/store";
import { Banner } from "./neon/Banner";
import { refreshSettings } from "../app/settings";

/** FakeSniBanner is shown on every page, in both modes, while Fake SNI
 * decrypts traffic. It cannot be dismissed (spec 2B 9.2). */
export function FakeSniBanner() {
  const { t } = useTranslation();
  const fs = useGhost((s) => s.snapshot.fakeSni);
  if (!fs?.active) return null;
  const view = () => {
    const st = useGhost.getState();
    st.setPage("fakesni");
    if (st.settings && st.settings.mode !== "full") st.setSettings({ ...st.settings, mode: "full" });
    void Service.SetMode("full");
  };
  return (
    <Banner
      tone="violet"
      actions={[
        { label: t("fakesni.view"), onClick: view },
        { label: t("fakesni.off"), onClick: () => void Service.SetFakeSNI(false).then(refreshSettings), primary: true },
      ]}
    >
      {t("fakesni.banner", { count: fs.domains })}
    </Banner>
  );
}
