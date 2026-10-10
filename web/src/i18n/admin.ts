import type { Loaders } from "./index";
import { withKinkan, type KinkanDict } from "./kinkan";

// The admin panel reads every section. `Loaders` makes the compiler check en.json against
// ru.json's shape: a key one file lacks fails the build.
export const adminDicts: Loaders = {
  ru: withKinkan(() => import("./ru.json"), () => import("./kinkan.ru.json")),
  en: withKinkan(() => import("./en.json"), () => import("./kinkan.en.json")),
};

// The same check for the fork's texts: kinkan.en.json has every key of kinkan.ru.json.
export const kinkanEnShape: () => Promise<{ default: KinkanDict }> = () => import("./kinkan.en.json");
