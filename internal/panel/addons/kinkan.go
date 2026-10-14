package addons

// kinkanCatalogKey is Mikan's release key. The marketplace (getmikan/marketplace) is
// Mikan's and signs its catalog with that key, while the fork signs its own releases with
// another one (release.PublicKey). Adapters still come only from ghcr.io/getmikan by
// digest.
const kinkanCatalogKey = "Z3wSIPBSaJxh5CsGO8eINI0aM0kyrQ46EcJSNeH85W8="
