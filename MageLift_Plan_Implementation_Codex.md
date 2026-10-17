# MageLift — Plan d’implémentation pour Codex

**Version du plan : 1.1 — 17 octobre 2026 — Builds et registres sous contrôle utilisateur**  
**Dépôt :** `magelift/magelift`  
**Base inspectée :** `wip/all-local-work`, commit `3ac1b844a7117752f3a5045054587508c16dcf6c`  
**Branche de travail proposée :** `reboot/community-alpha`

## Révision 1.1 — Décision sur les images et la CI/CD

Cette version **remplace intégralement la version 1.0**. Le changement concerne la fabrication et la distribution des images, pas l’architecture cloud ni l’ordre GCP → AWS.

**Décision du mainteneur : aucune image Docker MageLift à publier ou maintenir dans un registre central, sur GHCR ou ailleurs.** MageLift distribue des recettes de construction et du code versionnés. Les images sont construites par la CI/CD du projet utilisateur et poussées dans un registre contrôlé par celui-ci : GHCR, Amazon ECR, Google Artifact Registry ou autre registre compatible et accessible.

Les images amont (PHP, Composer et services, par exemple) restent utilisables avec une version/digest résolus. Les stages builder/runtime spécifiques à MageLift sont construits sur le runner utilisateur ; leur publication séparée n’est pas nécessaire. Un cache externe utilisateur est une optimisation, jamais une dépendance nécessaire au build.

**Conséquence de priorité : un workflow CI utilisateur minimal, exécuté réellement, fait partie du jalon A.** Seules la multiplication des plateformes CI et les fonctionnalités CI avancées sont reportées. Un build local reste possible pour développer et diagnostiquer, avec la même recette.

Les observations de code sont issues d’une lecture, sans exécution nouvelle des tests ni déploiement pour cette révision. Les commandes, champs et fichiers ajoutés ci-dessous sont des objectifs d’implémentation, pas une affirmation qu’ils existent déjà.

## 0. Mandat et décisions

Livrer un outil open source utilisable par la communauté, puis améliorer ses capacités à partir des retours. Il ne s’agit ni d’un hébergeur managé ni d’un programme de certification de toutes les architectures possibles.

Le mainteneur dispose, selon ses indications, de crédits GCP illimités et d’environ 150 USD de crédits AWS. GCP est donc le terrain principal de développement et de tests réels. AWS reçoit des tests ciblés. Ces indications ne prouvent ni la couverture de chaque service par les crédits ni les permissions disponibles : les vérifier avant les mutations, sans exposer de secrets.

### Décisions arrêtées

- Continuer le code existant ; aucune réécriture complète, aucune branche orpheline.
- Garder Go/Cobra, YAML, Pulumi Automation API, le build PHP et les images OCI.
- Distribuer les recettes de build, pas des images MageLift préconstruites. Aucun `FROM ghcr.io/magelift/...` ni téléchargement obligatoire d’image appartenant au projet MageLift sur le parcours normal.
- Construire dans la CI/CD utilisateur ; publier images, caches et preuves dans ses espaces. Le choix du registre est indépendant du cloud de déploiement, sous réserve de capacité/authentification explicites.
- Garder le build local comme outil de diagnostic ; pas de service de build hébergé imposé et pas de Docker installé dans les nœuds de production pour fabriquer l’application.
- Garder le monorepo, le manifeste d’artefact existant, le contrat des opérations et le plugin GCP signé.
- Une première recette GCP GKE Autopilot ; conserver AWS ECS et les providers expérimentaux, sans développer leur matrice entière.
- Publier une alpha communautaire sans attendre la console, une API de plugins stable, tous les brokers, la HA ou toutes les restaurations.
- Conserver les mécanismes de sécurité utiles. Simplifier les obligations de certification et de planification, pas la protection des données et des ressources.
- Ne pas exiger un nouveau spec/plan/approval pour chaque sous-tâche de ce document lorsqu’il est transmis par le mainteneur comme instruction d’exécution. Réconcilier cette règle avec `AGENTS.md` dans ML-00. Les validations explicites pour publication, dépense hors enveloppe et actions destructrices protégées restent applicables.

**Principe de travail : corriger une capacité déjà présente avant d’en créer une deuxième version.** Une case cochée dans l’ancien plan est un élément à vérifier, pas une invitation à réimplémenter.

### Ce plan ne commande aucune opération immédiate dans les comptes

Le présent document organise une future implémentation. Il ne contient aucun identifiant de compte autorisé, aucun secret et aucune autorisation de supprimer des ressources existantes. L’exécution doit utiliser une cible d’acceptance explicitement renseignée et reconnue. À défaut, avancer sur le code et les tests locaux ; marquer les essais cloud `NOT_RUN`.

## 1. État de départ utile

Ces observations proviennent de la lecture du dépôt ; les tests n’ont pas été réexécutés lors de la préparation du plan.

| Élément existant | Conséquence pour l’implémentation |
|---|---|
| `intent/ROADMAP.md` prévoit déjà « GCP alpha, AWS second, provider platform third ». [S1] | Mettre à jour cette roadmap ; ne pas créer une stratégie concurrente. |
| `intent/alpha-development-loop/plan.md` a déjà coché les corrections de fermeture des providers et l’ordre des migrations. Des vérifications finales restent ouvertes. [S2] | Reprendre les tâches réellement restantes, avec les tests de non-régression existants. |
| `internal/providerhost/` possède résolution, cache, téléchargement et contrôle de confiance. [S3] | Compléter l’acquisition transparente ; pas de nouveau gestionnaire de plugins. |
| `providers/gcp/` est un module/plugin séparé ; AWS reste embarqué. [S4] | Ne pas bloquer la première alpha sur l’extraction AWS. |
| Le build possède `ArtifactManifest`, `ImmutableArtifactContract`, les phases et les patches. [S5, S6] | Pas de second manifeste ni de second cycle de déploiement. |
| `internal/cli/build.go` exige actuellement `--image`, `--builder-image` et `--runtime-image` pour `--push`. [S19] | Retirer l’obligation de fournir des images de base préconstruites ; compléter le chemin source-build existant. |
| L’intent historique `published-build-images` proposait des images centrales. [S5] | Le marquer remplacé dans la roadmap ; réutiliser ses tests utiles, sans exécuter son objectif de publication. |
| `PatchLifecycle.php` appelle QPT et les hotfixes ; son chemin lu n’applique pas les cloud patches. [S6] | Ajouter une politique explicite, sans rejouer deux fois les patches. |
| Les skills utilisateurs et les sorties JSON existent. [S7, S8] | Les fiabiliser ; pas de chatbot obligatoire. |
| `make test` lance `go test -race ./...`, alors qu’`AGENTS.md` interdit un race global non borné. [S9] | Réconcilier les cibles de tests avant de demander systématiquement `make verify`. |

La présence d’un fichier ne prouve pas qu’une fonctionnalité marche. Inversement, une limite dans un ancien rapport ne prouve pas que le code courant contient toujours le défaut.

## 2. Résultats à livrer

### Jalon A — Alpha communautaire GCP

Une personne disposant d’un dépôt applicatif, d’une CI avec Docker/BuildKit, d’un registre et d’un accès cloud correctement préparés peut :

1. Installer un binaire publié, sans compiler MageLift ni cloner son dépôt.
2. Configurer un projet Magento par YAML ; obtenir des erreurs exploitables.
3. Déclencher sa CI pour construire l’image applicative et ses stages intermédiaires depuis les recettes versionnées, appliquer les patches déclarés et publier dans son propre registre.
4. Déployer depuis cette CI le digest produit sur la recette GCP documentée et accéder à un Magento fonctionnel en HTTPS.
5. Lire l’état et les logs, exécuter une commande applicative, redéployer et promouvoir un artefact existant sans relancer sa construction.
6. Détruire son environnement jetable sans supprimer des ressources étrangères.
7. Ouvrir une issue reproductible ou une PR à partir d’instructions utilisables.

L’alpha peut contenir des défauts documentés et des capacités expérimentales. Elle ne doit pas présenter comme fonctionnel un parcours de démarrage connu pour échouer systématiquement.

Le développeur peut aussi exécuter la même recette sur son poste avec Docker pour reproduire un problème. Le build local n’est pas une étape manuelle obligatoire avant chaque release CI.

### Jalon B — Exploitation et deuxième provider

Livrer des préversions incrémentales : sauvegarde/restauration bornée, previews gérées, API CLI exploitable par les agents, console locale en lecture seule et AWS utilisant le contrat commun. Ces éléments ne sont pas une condition rétroactive de publication du jalon A.

### Trois notions distinctes dans les docs

- **Maintenance :** first-party ou community.
- **Maturité :** alpha ou experimental, par capacité/recette.
- **Dernier test réel :** commit, date, configuration, opérations effectuées et résultat ; `NOT_RUN` reste un résultat honnête.

Ne pas développer un nouveau moteur de badges ou un nouveau registre pour cela. Adapter les sources existantes de `docs/capability-matrix.md` et `docs/evidence/`. Préserver les rapports historiques sans réécrire leurs conclusions.

## 3. Recette de référence

Reprendre celle de la roadmap existante, plutôt que réinventer l’infrastructure. [S1]

| Composant | Décision initiale |
|---|---|
| Magento | Magento Open Source 2.4.9 comme point de départ du dépôt ; vérifier les correctifs de sécurité applicables avant exposition publique. |
| PHP / Composer | Tuple déjà verrouillé et compatible avec cette recette ; aucune mise à jour globale « latest ». |
| Runtime | GCP GKE Autopilot, nginx/PHP-FPM. |
| Base | Cloud SQL MySQL, version explicitement vérifiée avec Magento. |
| Cache / sessions | Memorystore Valkey, version du tuple vérifié. |
| Recherche | Chemin OpenSearch existant, une réplique, avec indexation et recherche effectivement exercées. |
| Queue | Queue base de données existante pour le parcours alpha ; ne pas prétendre qu’elle couvre tous les besoins/extensions. |
| Médias | GCS privé via le contrat d’adaptation déjà implémenté ; ce contrat doit être testé, pas supposé natif. |
| Secrets | Références Secret Manager et matérialisation au runtime. |
| Déploiement | Job candidat/migrations avant activation de la nouvelle release. |
| Accès | Chemin HTTPS existant, sans introduire un nouveau CDN ni une nouvelle WAF pour fermer l’alpha. |
| Diagnostic | Logs natifs, état de déploiement et erreurs exploitables. |
| Construction | Runner CI du projet ; recette MageLift versionnée et stages construits sur ce runner. |
| Registre de référence GCP | Artifact Registry du compte utilisateur, ou registre existant explicitement choisi ; ce choix ne verrouille pas les autres cibles. |
| Cache | Backend BuildKit du projet, externe si disponible, privé s’il contient du code ou des dépendances propriétaires. |

Ne pas déclarer une compatibilité versionnelle à partir du seul démarrage d’un conteneur. Vérifier le tuple dans la documentation Adobe avant de le publier. En cas de contradiction, limiter la correction à la recette et à ses tests ; ne pas mettre à jour tous les providers.

La cible AWS conserve son architecture ECS existante. Reprendre la même image applicative uniquement si l’architecture CPU et le contrat runtime le permettent ; injecter les bindings AWS au runtime. Aucune hypothèse implicite « la preuve GCP valide AWS ».

## 4. Ordre des lots

| Lot | Objet | Dépendances | Bloque le jalon A ? |
|---|---|---|---|
| ML-00 | Reprise, règles et boucle de tests cohérente | — | Oui |
| ML-01 | Fermer la boucle de développement GCP existante | ML-00 | Oui |
| ML-02 | Installation et acquisition transparente des outils/providers | ML-01 | Oui |
| ML-03 | Recettes source-build, registre utilisateur et CI minimale | ML-02 | Oui |
| ML-04 | Patches et qualité des erreurs de build | ML-03 | Oui pour QPT/hotfixes ; cloud patches bornés, sans matrice exhaustive |
| ML-05 | Parcours GCP de référence et publication communautaire | ML-01 à ML-04 | Oui |
| ML-06 | Opérations quotidiennes, sauvegarde et previews | ML-05 | Non |
| ML-07 | AWS ciblé puis extraction du provider | ML-05 ; primitives ML-06 seulement si nécessaires | Non |
| ML-08 | Interface machine/skills et console locale | ML-06 ; UI GCP sans attendre ML-07 | Non |
| ML-09 | Contribution d’un provider et intégrations externes minimales | ML-07 | Non |

Un lot peut produire plusieurs petites PRs, mais il n’y a qu’un seul lot d’implémentation actif par défaut. Ne pas lancer dix agents qui modifient simultanément le SDK, le registre et les workflows.

---

## ML-00 — Reprendre le projet sans nouvelle boucle d’audit

**But :** disposer d’une base exécutable et d’une seule définition des priorités.

**Zones :** `AGENTS.md`, `intent/ROADMAP.md`, `intent/alpha-development-loop/plan.md`, `Makefile`, configuration CI, docs de contribution.

### Travail

1. Vérifier `git status`, la branche et le SHA. Ne pas écraser le travail local. Créer `reboot/community-alpha` à partir de la branche de travail courante. Si elle a avancé, lire le diff depuis le SHA inspecté et enregistrer la nouvelle base.
2. Lire l’ancien plan actif et les zones de code concernées. Produire une liste courte : déjà fait / défaut reproduit / restant. Pas de nouvel audit général de tous les clouds.
3. Mettre `intent/ROADMAP.md` en accord avec les lots de ce plan. Mettre en pause les anciennes exigences hors périmètre, sans les marquer terminées. Marquer `published-build-images` remplacé par ML-03 ; retirer la publication d’images centrales et remonter la CI minimale dans le jalon A. Ne pas supprimer les sources Docker ni réécrire les preuves historiques.
4. Adapter `AGENTS.md` et les règles de release : validation pragmatique, statut expérimental explicite, absence d’obligation de certifier les intersections.
5. Corriger la contradiction entre `make test`, `make verify` et les instructions mémoire/race. Préserver les tests, mais séparer test rapide, vérification de la recette et race ciblé. Ne pas retirer un test uniquement parce qu’il est rouge.
6. Ajouter des cibles **proposées** `make test-fast` et `make verify-community`, ou réutiliser leurs équivalents exacts s’ils existent. Documenter une seule commande canonique par usage.
7. Couvrir séparément le module racine, `sdk/` et `providers/gcp/`. Un `go test ./...` à la racine n’est pas un substitut aux tests des modules imbriqués. Utiliser le workspace là où le couplage existant le nécessite ; ne pas exiger immédiatement `GOWORK=off` pour GCP.
8. Faire fonctionner les tests PHP dans un environnement reproductible lorsque le poste hôte manque d’extensions. Réutiliser les images/outils existants.

### Vérification

- Tests ciblés sans credentials et sans dépenses cloud.
- Cas nominal et cas d’échec d’une cible Make : un échec réel produit un code non nul.
- Les commandes documentées couvrent effectivement les trois modules, ou indiquent précisément lesquels elles excluent.
- Les anciennes suites larges restent accessibles ; elles ne bloquent pas l’alpha pour des providers gelés sans rapport avec une PR.

**Fin :** base enregistrée, workflow local cohérent, liste du reste à faire limitée aux lots. Aucun déploiement, nouveau framework de tests ou changement de langage.

## ML-01 — Terminer la boucle de développement GCP existante

**But :** corriger et tester le produit sans publier une nouvelle préversion publique à chaque essai.

**Zones :** `scripts/development-loop/`, `.github/workflows/alpha-development-loop.yml`, `providers/gcp/scripts/gcp-acceptance-local.sh`, `internal/providerhost/`, `internal/cli/execute.go`, tests associés. [S2]

### Travail

1. Réexécuter les tests existants concernant `CachedDialer.Close`, fermeture des providers sur retour/erreur/annulation et ordre `setup:upgrade` avant `app:config:import`.
2. Ne réécrire ces mécanismes que si une régression est reproduite. L’ancien plan les décrit déjà corrigés.
3. Reprendre les vérifications ouvertes de `alpha-development-loop` : distribution, artefact et produit GCP comme résultats distincts.
4. Réutiliser les canaries adressés par commit et les artefacts immuables. Aucun nouveau format de release, de manifeste ou de preuve cryptographique.
5. Un nouveau tag canary, si le workflow existant en a besoin, est préparé pour le mainteneur ; il n’est pas poussé implicitement. Le développement local ne dépend pas de ce tag.
6. Paramétrer le projet GCP d’acceptance sans valeurs codées en dur. Vérifier l’identité courante, les permissions, les APIs utiles et le préfixe d’appartenance avant mutation.
7. Ajouter ou corriger uniquement le contrôle nécessaire pour qu’un échec d’une étape n’efface pas les résultats des autres.
8. Garder les logs bruts ignorés par Git et produire un résumé expurgé : commit, artefact, opérations, résultat, restes connus.

### Politique GCP

Déployer une recette de petite taille dans un projet d’acceptance. Les crédits permettent de tester le vrai service plutôt que développer un émulateur supplémentaire.

Par défaut, nettoyer après le run. Un environnement de debug peut être conservé **uniquement via une option explicite**, avec propriétaire, date d’expiration, inventaire et procédure de destruction. Ne pas créer une infrastructure permanente implicite. La fermeture de la CLI ne doit jamais être l’unique mécanisme de nettoyage d’un run interrompu.

### Vérification

- Succès, erreur et annulation ne laissent pas de processus provider orphelin.
- Résultats `PASS`, `FAIL`, `SKIP`/`NOT_RUN` corrects et indépendants.
- Un test exécuté avec un double n’est jamais présenté comme un essai GCP réel.
- Si credentials disponibles : un run réel avec diagnostic et inventaire de sortie. Un HTTP 500 connu est corrigé ou reste `FAIL`, pas reclassé comme succès du produit.

**Fin :** les défauts peuvent être diagnostiqués et corrigés depuis un commit sans multiplier les releases publiques. Ne pas rouvrir la matrice de certification.

## ML-02 — Rendre l’installation et les providers transparents

**But :** supprimer le travail manuel d’assemblage de la toolchain pour l’utilisateur.

**Zones :** `internal/providerhost/{resolve,fetch,download,lock}.go`, `internal/toolchain/`, `internal/registry/`, `internal/cli/dependencies.go`, installateur, `.goreleaser.yaml`, workflows de release. [S3]

### Travail

1. Réutiliser le resolver unique. Ajouter l’acquisition du provider dans le chemin commun d’activation de la cible, après validation de l’enveloppe et sélection explicite de l’environnement.
2. Priorité : lock projet valide, puis version first-party épinglée par la release. Ni `latest` implicite ni substitution de provider après erreur.
3. Installation atomique dans un cache versionné par OS/architecture. Gérer un téléchargement interrompu et deux processus concurrents.
4. Préserver la vérification du publisher, signature, digest, protocole et opérations requises. Une cache corrompue n’est pas silencieusement acceptée ou remplacée.
5. Garder l’installation explicite pour préchauffage/offline/réparation. Les providers communautaires ne sont pas téléchargés/exécutés automatiquement sur la seule foi d’un nom YAML.
6. `doctor`, `version`, `help` et la validation de l’enveloppe ne déclenchent pas de déploiement ou d’installation surprise. Lorsqu’une validation provider exige un plugin absent, l’indiquer explicitement.
7. Automatiser, via les composants existants, l’acquisition des outils requis par le chemin supporté. Docker et l’authentification du compte restent des prérequis clairement annoncés ; Go ne doit pas être requis chez l’utilisateur.
8. Ne pas inventer un moteur de package management. Employer les archives/outils déjà gérés et l’intégration existante aux gestionnaires de paquets.
9. Qualifier les archives Linux, Brew et Scoop. Pour Scoop, ajouter un manifeste/bucket seulement dans un espace possédé par le projet. Tester réellement Windows ; si un chemin cloud n’est pas exécutable nativement, limiter explicitement ce qui y est annoncé et documenter WSL.
10. Mesurer séparément archive CLI, binaire installé, provider, toolchain à la demande et images/cache Docker. Ne pas annoncer un poids total à partir de la seule archive compressée.

### Vérification

Machine propre sans source du framework ; pin projet prioritaire ; signature invalide refusée ; offline avec cache valide ; offline sans cache = instruction claire ; interruption sans cache partielle utilisable ; concurrence sans corruption.

**Fin :** le guide ne demande plus de placer un provider à côté du binaire à la main. Ne pas extraire AWS dans ce lot.

## ML-03 — Recettes de build, registre utilisateur et CI minimale

**But :** construire l’application et ses fondations conteneurisées dans la CI/CD du projet, sans image centrale MageLift, puis publier dans le registre choisi. MageLift reste responsable de la recette et de l’orchestration ; l’utilisateur ne doit pas maintenir manuellement une copie complexe du framework.

**Zones :** `images/`, `docker-bake.hcl`, `build/`, `internal/build/{kit,pipeline,plan,runner}/`, `internal/toolchain/`, `internal/cli/{build,ci}.go`, configuration/schema, `sdk/artifact.go`, workflows et onboarding. Réutiliser les contrats existants. [S5, S19]

### ML-03a — Livrer la recette avec le logiciel

1. Faire évoluer les Dockerfiles et scripts existants. Conserver la distribution Linux, les chemins, UID/GID, ports, extensions et modes runtime déjà utilisés par la recette GCP. Ne pas faire une migration Debian → Alpine ou introduire Supervisor seulement pour imiter une autre application.
2. Distribuer les templates Docker/Bake, le code PHP MageLift nécessaire et les fichiers runtime dans la release source/binaire versionnée : privilégier des fichiers embarqués via `go:embed`, sans image Docker embarquée. Le binaire installé doit contenir ce qui permet de rendre la recette sans cloner le dépôt MageLift. Réutiliser le mécanisme existant s’il répond déjà à cette exigence.
3. Matérialiser les fichiers générés dans un répertoire de travail propre, ignoré, identifié par la version/hash de recette. Permettre d’inspecter/exporter le plan de build pour le diagnostic ; pas de nouvel outil de templating ni de DSL.
4. Le parcours normal reste YAML-only. Les templates sont maintenus dans MageLift ; ne pas demander à l’utilisateur de copier puis synchroniser une arborescence de Dockerfiles à chaque mise à jour. Une override de Dockerfile/recette reste avancée et explicitement identifiée dans la traçabilité.
5. Utiliser des images amont de confiance résolues par digest. Verrouiller les outils téléchargés explicitement. Pas de `latest` résolu silencieusement au milieu d’un build de release. Une mise à jour des bases/outils produit un changement révisable ; l’ancien digest n’est pas modifié. [S15]
6. Ne pas confondre pinning des images et build totalement hermétique : les dépôts de paquets, extensions, scripts et horodatages peuvent varier. Enregistrer les entrées et versions installées ; ne pas exiger une infrastructure de snapshots de paquets pour fermer l’alpha.

### ML-03b — Construire sur le runner utilisateur

1. Adapter `buildPipelineRequest` et le chemin `--push` : **seule la destination finale doit être nécessaire au parcours source-build**, en plus de la configuration et des credentials utiles. `--builder-image` et `--runtime-image` deviennent des overrides avancées, pas des prérequis.
2. Construire les stages PHP/extensions, builder et runtime sur le runner, à partir des images amont et des recettes livrées. Ils peuvent être des stages d’un Dockerfile multi-stage ou des intermédiaires locaux dans le pipeline existant : choisir la plus petite modification correcte, pas un deuxième build engine.
3. Ne pas exiger un push des images builder/runtime intermédiaires pour les réutiliser dans le même build. Par défaut, publier seulement les images déployables nécessaires et, s’il est configuré, le cache.
4. Même exigence pour tout helper/sidecar spécifique à MageLift requis par la recette : le construire chez l’utilisateur ou supprimer la dépendance si le chemin ne l’utilise pas. Les images de services amont non modifiées peuvent être consommées directement, ou miroitées par l’utilisateur. Aucun helper oublié ne doit recréer une dépendance à GHCR MageLift.
5. Réutiliser le lifecycle PHP : validation → Composer install → patches → DI → contenu statique → packaging. Préserver l’inventaire des modules et leur activation déclarée. Ne pas écrire une deuxième implémentation shell de ces phases dans le Dockerfile.
6. Résoudre les difficultés de démarrage de Composer sur source fraîche : une vérification exigeant `vendor/` ne doit pas précéder son existence ; une vérification sur lock peut précéder l’installation si l’outil le permet. Ajouter la régression au bon niveau.
7. Exclure `auth.json`, `app/etc/env.php` runtime, `.env` sensibles, états cloud, logs et dumps du contexte. Utiliser les secrets BuildKit pour Composer/private Git et le mécanisme de références existant ; aucun secret via `ARG`, `ENV` persistant ou `COPY`. Vérifier aussi les sorties des scripts et les couches intermédiaires, pas seulement le filesystem final. [S14]
8. Accepter des credentials provenant de la CI utilisateur, pas seulement d’un secret store cloud. Une CI authentifiée à son registre ne doit pas être obligée d’accéder à Pulumi ou à la base pour construire. Le build peut fonctionner sans adopter le provisioning MageLift.
9. Choisir une plateforme explicite, Linux/amd64 pour la recette GCP initiale. Retirer le multiarchitecture implicite coûteux du parcours par défaut. Linux/arm64 ou un index multiarchitecture restent opt-in ; enregistrer index et manifeste de plateforme sans comparer leurs digests comme s’ils étaient identiques.
10. Conserver le manifeste externe existant, finalisé après obtention du digest. Ajouter seulement les métadonnées manquantes : recette, bases amont, plateforme, patches et versions. Ne pas rebâtir l’image pour lui injecter son propre digest final.
11. Faire fonctionner le build local avec la même recette. Un cache vide ou un runner neuf ne doit jamais nécessiter une ancienne image produite par MageLift.

### ML-03c — Cache sous contrôle utilisateur

1. Conserver les stages coûteux stables avant les sources applicatives : OS, PHP/extensions, Composer/dépendances lorsque leur contrat le permet, puis code/patches/DI/static. Toute dépendance réelle d’une phase doit participer à son invalidation. [S20]
2. Réutiliser BuildKit et son cache externe `type=registry` ; configuration facultative de `cache-from`/`cache-to`, références distinctes de l’artefact final. Le mode `max` peut conserver les stages intermédiaires sans les publier comme images de base. [S13]
3. Distinguer cache de couches exporté et contenu des `RUN --mount=type=cache`. Ne pas promettre que ce dernier est automatiquement restauré sur un runner éphémère par le seul export registry. Mesurer le bénéfice réel avant toute optimisation supplémentaire.
4. Isoler les caches par projet, architecture/recette et frontière de confiance. Une PR de fork n’obtient ni credentials de lecture de caches privés ni droits d’écriture sur le cache de release. Ne pas exécuter du code non revu sur un runner persistant privilégié.
5. Considérer les couches intermédiaires comme sensibles lorsqu’elles contiennent du code privé ou des dépendances propriétaires. Ne pas exposer le cache publiquement par défaut, même si l’image finale retire certains fichiers.
6. Un cache manquant déclenche un build froid, pas un blocage. Un échec d’authentification du registre de publication ou un échec réel du build reste fatal. Les politiques d’erreur d’export cache sont explicites.
7. Mesurer build froid et build chaud sans inventer de gain chiffré. Ne pas introduire une fabrique centrale d’images pour optimiser ce lot.

### ML-03d — Registre indépendant de la cible cloud

1. Accepter une référence OCI de destination configurable et une configuration de cache séparée. Ne pas réécrire un chemin GHCR utilisateur vers Artifact Registry/ECR parce que `target.provider` a changé.
2. Réutiliser l’authentification Docker/Buildx existante, ses credential helpers et les actions d’authentification CI. Limiter les adaptateurs spécifiques à ce qui est nécessaire ; aucun nouveau framework de registry providers.
3. Séparer **push CI**, **pull runtime**, **provisionnement du dépôt** et **conservation des artefacts**. Une CI pouvant pousser ne prouve pas que GKE/ECS peut tirer l’image après expiration du job.
4. Registre existant : référence sans adoption ni suppression implicite. Registre créé sur demande : bootstrap explicite, propriété enregistrée et politique de rétention. La création du registre/IAM ne doit pas dépendre d’une application déjà construite dans ce même registre.
5. Référence initiale GCP : Artifact Registry du projet utilisateur ; configurer l’identité de pull GKE et ses permissions. Ne pas supposer que l’identité applicative du Pod suffit au pull effectué par le nœud/runtime. [S18]
6. Chemin registry privé générique : configuration Docker existante côté build ; `imagePullSecrets`/authentification runtime équivalente côté Kubernetes. En ML-07, ECS distingue ECR natif et `repositoryCredentials` pour les registres externes. [S16, S17]
7. Gérer références de secrets, portée et renouvellement. Un jeton de CI temporaire n’est pas stocké tel quel comme credential durable de production. Une combinaison cross-cloud dont le renouvellement n’est pas implémenté est marquée indisponible/expérimentale avec explication, pas masquée en rendant l’image publique.
8. Vérifier négociation OCI, architecture, compression et transport des attestations pour le chemin annoncé. Le support générique du push ne signifie pas que toutes les fonctions de tous les registres sont testées.
9. Conserver la provenance, les SBOM et les signatures par les mécanismes existants, mais au nom du **producteur CI utilisateur**. La confiance de la signature d’un binaire provider MageLift et celle d’une image applicative utilisateur sont deux politiques distinctes. Pas de bypass silencieux si le registre ou l’issuer ne correspond pas au contrat configuré.
10. Conserver les digests encore déployés, les candidats validés et la fenêtre de rollback annoncée. Supprimer une preview ne doit pas supprimer une image partagée utilisée ailleurs. La collecte de cache n’est pas la collecte des releases.

### ML-03e — Une CI utilisateur minimale dès l’alpha

1. Réutiliser les commandes/générateurs CI existants. Livrer d’abord un workflow GitHub Actions lisible : checkout → installation versionnée de MageLift → Buildx → authentification → build/push → conservation du manifeste/digest → déploiement du digest sur l’environnement autorisé.
2. Le workflow appartient au dépôt utilisateur et utilise ses runners, identités, secrets et registre. La CLI reste invocable depuis GitLab CI, Jenkins ou autre runner compatible sans backend GitHub obligatoire ; ne pas prétendre leurs templates testés sans les avoir exécutés.
3. Préparer explicitement, une fois, la fédération d’identité et les permissions dans la cible autorisée. Rôles de build, déploiement et pull runtime distincts et limités. Un job de PR non approuvé ne reçoit pas les secrets ni les droits de déploiement.
4. Exécuter les commandes en mode non interactif avec sélection de l’environnement. Une information manquante échoue clairement ; aucun choix implicite de production.
5. Faire que le job de déploiement consomme le digest/manifest du job de build ; un job de promotion consomme une release déjà construite sans invoquer BuildKit. `staging` et `production` sont des configurations runtime, pas deux recettes de build.
6. Une séparation de registres/comptes peut nécessiter une copie d’artefact. Ne pas reconstruire ; préserver et vérifier les manifests/blobs, transporter les preuves utiles et refuser toute transformation silencieuse. Si l’identité change, ne pas annoncer « même digest ».
7. Pour l’alpha, tester un workflow complet GCP et un registre OCI de test pour le chemin générique. ECR réel est vérifié dans le budget ML-07 ; les combinaisons non exécutées restent `NOT_RUN`. Pas de matrice toutes CI × tous registres × tous clouds.
8. Les workflows du projet MageLift continuent à **construire et tester** ses recettes. Désactiver leur publication centrale d’images Docker ; ne pas supprimer les tests et ne pas republier ces images sous un autre namespace. La distribution signée de la CLI et de ses providers continue.

### Vérification

- Runner neuf, sans checkout du framework et sans accès aux images Docker MageLift : build/push complet depuis une release de la CLI et le dépôt synthétique de la boutique.
- Build local froid et build CI avec la même recette ; versions et entrées traçables. Deux builds peuvent différer bit à bit ; une promotion, elle, réutilise l’artefact produit sans rebuild.
- Build chaud : cache utilisé ; changement PHP/extension/lock/patch invalide les étapes nécessaires ; aucun cache ne fait disparaître un patch modifié.
- Secrets fictifs absents des couches, caches exportés, logs, provenance et manifeste ; contexte nettoyé ; tentatives de PR non autorisées refusées.
- CI GCP réelle : publication dans Artifact Registry utilisateur, pull par la cible avec ses propres credentials et HTTP utile ; pas de dépendance à GHCR MageLift.
- Registre OCI local/générique : push, digest, cache, échec d’authentification explicite ; les fonctionnalités non implémentées ne renvoient pas de faux succès.
- Promotion sans appel au builder ; aucun effacement d’image référencée par une autre release/environnement.
- Les fonctionnalités de signature/attestation exposées doivent être vérifiées sur le chemin annoncé ; ne pas désactiver la vérification pour fermer le lot.

**Fin :** le développeur modifie le YAML et son code, la CI construit toutes les parties spécifiques à MageLift, publie les images nécessaires dans son registre et transmet leurs digests au déploiement. Aucun catalogue central d’images Docker MageLift à publier. Pas de nouvelle matrice Magento/PHP/CI/registre.

## ML-04 — Rendre les patches fiables et compréhensibles

**But :** appliquer automatiquement les patches déclarés, pas tous les patches disponibles.

**Zones :** `build/src/Magento/{PatchLifecycle,PatchApplier,PatchCommand,LifecyclePlan}.php`, classes/tests de configuration, configuration Go, générateur de schéma et manifeste. [S6]

### Travail

1. Conserver l’appel au Quality Patches Tool fourni par les dépendances du projet. Ne pas recopier son catalogue ou son moteur. [S10]
2. Vérifier que la source Composer verrouillée et le runtime de build apportent l’outil requis. Erreur explicite si une liste est configurée sans outil disponible.
3. Définir un ordre déterministe : éventuelle politique cloud explicite → QPT sélectionné → hotfixes locaux triés.
4. Ajouter l’intégration cloud patches comme **opt-in expérimental**, jamais « appliquer tous les cloud-required hors Adobe Cloud » par défaut. Le comportement des commandes du package upstream verrouillé doit être lu et testé avant de coder l’adaptateur.
5. Ne pas supposer que `ece-patches apply` accepte une liste d’identifiants ou applique seulement une catégorie. Si l’outil orchestre déjà QPT et hotfixes, éviter leur deuxième application. En l’absence d’une sélection publique correctement exploitable, limiter l’opt-in au comportement upstream réellement documenté et afficher sa portée ; sinon refuser clairement cette option et la documenter comme non disponible. Aucun faux support.
6. Ne pas bloquer toute l’alpha sur une matrice exhaustive de cloud patches. QPT et hotfixes doivent fonctionner. La sous-capacité cloud reste expérimentale ou explicitement indisponible tant que son adaptateur borné ne passe pas son test.
7. Réutiliser les informations upstream de dépendances, remplacement et applicabilité, sans parser du texte humain avec des heuristiques fragiles si une API/donnée structurée est disponible. Ne pas créer une base parallèle.
8. Conserver QPT/package versions et hashes des hotfixes dans la traçabilité existante. Étendre le manifeste additivement seulement si nécessaire ; ne pas ajouter un second patch lock dupliquant `composer.lock` et le manifeste.
9. Gérer correctement « déjà appliqué » : l’accepter seulement si l’état est identifié sans ambiguïté. Un patch partiellement appliqué ou conflictuel fait échouer le build. Ne pas continuer sur simple warning.
10. Améliorer les messages : patch concerné, catégorie, version Magento, étape et action proposée, sans secret.
11. Ne pas ajouter des mises à jour automatiques silencieuses du catalogue pendant un build. Une mise à jour est une modification de dépendances à réviser.

### Vérification

QPT applicable ; identifiant inconnu ; package manquant ; hotfix compatible ; conflit ; déjà appliqué ; ordre stable ; doublon ; configuration erronée ; chemins malveillants ; cloud option désactivée ; cloud option activée avec comportement prouvé ou refus précis.

Employer des fixtures synthétiques pour les tests unitaires et un exemple upstream autorisé pour l’intégration. Les droits amont restent applicables ; vérifier `NOTICE`, `LICENSE` et `make check-clean-room`.

**Fin :** le build ne peut pas annoncer une application réussie après avoir ignoré un conflit. Pas de moteur d’auto-sélection par IA.

## ML-05 — Fermer le parcours GCP et publier l’alpha

**But :** une version utilisable par quelqu’un d’autre, pas une certification générale.

**Zones :** `providers/gcp/`, `internal/deploy/`, `internal/platform/`, `internal/cli/`, `scripts/development-loop/`, `docs/onboarding.md`, README, docs de support, release.

### Travail

1. Faire le parcours depuis une archive/canary dans une CI utilisateur propre, avec les recettes livrées et le registre du projet. Aucun pull d’image Docker MageLift centrale. Utiliser un projet Magento synthétique et un tuple exact.
2. Mettre les commandes d’onboarding en accord avec l’aide réelle. Conserver la sémantique actuelle de `magelift preview` comme plan d’infrastructure : ne pas la recycler en commande de création d’environnement éphémère.
3. Assurer l’ordre : config/identité → prérequis → bootstrap explicite (registre/IAM si demandé, puis état/infrastructure nécessaires) → build/push dans la CI utilisateur → plan → migrations contrôlées → activation du digest → état applicatif. Aucun besoin de credentials DB/production pour le job de build.
4. Vérifier le verrou de déploiement, le traitement d’une erreur et l’absence de lancement simultané des migrations par chaque réplique.
5. Les logs doivent permettre de distinguer échec Pulumi, Job de migration, rollout et erreur Magento. Conserver stdout propre en sortie JSON.
6. Tester HTTPS, une réponse applicative utile, un asset statique, création/lecture de données synthétiques, indexation/recherche, cron, queue DB et upload/lecture d’un média. Un `/health` statique ou « pod running » ne suffit pas pour ce scénario.
7. Vérifier les interdictions d’accès aux secrets/fichiers sensibles et aux médias privés concernés ; ne pas rendre le bucket public pour obtenir un test vert.
8. Faire un deuxième déploiement sur la même base. Changer un élément observable de l’application et vérifier la persistance des données précédentes. Vérifier séparément qu’une promotion de release consomme le digest existant sans build ; le test peut utiliser deux environnements jetables, sans promettre un profil HA de production.
9. Détruire uniquement les ressources explicitement possédées par le run. Conserver l’état tant qu’il reste des ressources à gérer. Les sauvegardes retenues, clés à suppression différée ou ressources partagées doivent être distinguées des fuites.
10. Faire un inventaire de sortie indépendant du seul code retour de Pulumi. Signaler les restes avec une procédure ciblée ; jamais de suppression « tout ce qui commence par magelift » dans le compte.
11. Corriger docs, captures de commandes, limitations et statuts. Ne pas promouvoir une réussite GCP comme preuve AWS.
12. Préparer les artefacts et notes de release. Le mainteneur approuve le tag et la publication ; pas de push de tag caché dans un test.

### Seuil de publication

Exiger un passage propre depuis la CI utilisateur et un deuxième déploiement fonctionnel sur la recette annoncée, plus installation/distribution/build/push/pull cohérents et promotion sans reconstruction. Si une correction change le scénario concerné, le rejouer. Il n’est pas nécessaire de répéter deux créations complètes uniquement pour remplir une obligation documentaire ancienne.

**Bloquants :** fuite de secrets, téléchargement non vérifié introduit par le lot, suppression hors périmètre, corruption reproduite du parcours nominal, migrations concurrentes, incapacité systématique à installer/build/déployer la recette annoncée.

**Non-bloquants s’ils sont documentés et non vendus comme acquis :** absence de HA, provider tiers non testé, broker alternatif, restore avancé, console, variations de performance, absence de test de toutes les versions et API de plugin encore alpha.

Ne pas publier « prêt pour toutes les productions ». Publier « alpha communautaire — recette testée et limites documentées ». La responsabilité d’exploitation reste au déployeur ; les sauvegardes et restaurations ne sont garanties que pour les opérations explicitement testées.

### Communauté

- Adapter `SUPPORT.md`, `CONTRIBUTING.md`, `SECURITY.md` et les templates existants, sans les dupliquer.
- Issue : version, OS, recette/provider, reproduction, résultat attendu/obtenu, diagnostic expurgé.
- PR : description ciblée, test de non-régression, coût cloud éventuel, évolution de contrat éventuelle.
- Canal privé pour les vulnérabilités ; pas de dumps ni de credentials dans une issue publique.
- Engagement proposé : prise en compte rapide et transparente, avec priorité à perte de données, sécurité et blocage de déploiement. Un objectif de première réponse sous deux jours ouvrés peut être adopté par le mainteneur ; ce n’est ni un délai garanti de correction ni un SLA d’hébergement.
- Pas de fermeture automatique prématurée des bugs reproductibles par un bot « stale ».

**Fin :** préversion installable, guide exécutable littéralement, recette bornée et contribution accessible. Publier avant d’entreprendre ML-06 à ML-09 ; ces lots ne doivent pas devenir une nouvelle condition de sortie.

## ML-06 — Opérations quotidiennes, sauvegarde et previews

**But :** rendre utiles et sûres les capacités existantes au quotidien.

**Zones :** `internal/cli/env.go`, `health.go`, `exec.go`, commandes de logs/données existantes, `internal/dumpimport/`, `internal/mediasync/`, `internal/releasejournal/`, `internal/automation/`, adaptateurs GCP.

### Travail

1. Inventorier les commandes déjà enregistrées et améliorer leurs chemins existants. Ne pas créer des synonymes concurrents `db dump`/`env dump` ou `status`/`health` sans nécessité.
2. Priorité : état et release active, logs bornés, exec, accès à la DB, dump/import, synchronisation contrôlée des médias.
3. Distinguer dans la documentation dump logique, sauvegarde gérée et restauration. Premier objectif : restaurer un jeu synthétique dans un environnement **nouveau et isolé**, puis y démarrer Magento.
4. Préserver les secrets nécessaires au déchiffrement applicatif par un mécanisme sécurisé ; ne jamais les joindre au dump public/diagnostic. Vérifier au moins un objet synthétique chiffré si ce mécanisme est revendiqué.
5. Refuser une restauration destructive sur un environnement protégé sans confirmation explicite de la cible et sauvegarde préalable appropriée. Documenter les conséquences des migrations de schéma ; ne pas appeler « rollback » la seule sélection d’une image ancienne.
6. Stabiliser création/liste/suppression des previews par l’identité déjà implémentée. Une preview initiale peut avoir des services dédiés ; ne pas inventer du multi-tenant partagé pour optimiser avant d’avoir mesuré.
7. Données synthétiques par défaut. Les copies de données réelles exigent une procédure d’anonymisation réellement adaptée au schéma et aux extensions ; ne pas promettre une anonymisation générique complète.
8. Neutraliser emails, paiements et intégrations sortantes dans les fixtures/previews de test. Les flags applicatifs ne prouvent pas l’isolation de toute extension : compléter par un contrôle réseau/credentials approprié lorsqu’il est nécessaire.
9. Brancher la purge des environnements expirés sur un scheduler/CI existant ou un workflow minimal, avec lock, dry-run, vérification de propriété et exclusions des environnements protégés. Le laptop fermé ne doit pas empêcher la purge.
10. Ne pas ajouter de calculatrice universelle de coût. Afficher la taille et les ressources connues ; un prix estimé reste daté et distinct d’une facture.

### Vérification

Dump puis restore synthétique avec assertions sur lignes/données/médias, mauvais environnement refusé, droits insuffisants, exec annulé, deux previews isolées, une preview expirée supprimée et un environnement protégé conservé.

**Fin :** publier les opérations au fur et à mesure, en indiquant lesquelles sont testées. La HA, le PITR universel, la reprise inter-région et l’anonymisation universelle restent hors lot.

## ML-07 — AWS : test ciblé, puis provider autonome

**But :** préserver AWS et le faire progresser sans consommer les crédits dans une matrice complète.

**Zones :** `internal/cloud/aws/`, `internal/registry/`, `internal/providerhost/`, `providers/gcp/` comme référence de protocole, `sdk/`, scripts AWS existants, release. Future destination proposée : `providers/aws/`.

### ML-07a : stabiliser le chemin existant

1. Exécuter tests unitaires, graphes Pulumi et tests d’émulateur existants, sans dépenses.
2. Préparer une seule recette AWS ECS à partir de l’existant. Lire les versions et contraintes réelles ; ne pas transposer littéralement les ressources GCP.
3. Avant toute création, établir coût indicatif, durée maximale, ressources et plan de nettoyage. Appliquer les règles budgétaires de la section 5.
4. Sur un run réel groupé : bootstrap explicite du registre/IAM si demandé → build/push de la recette dans ECR utilisateur (ou copie du même artefact compatible sans reconstruction) → déployer le digest → HTTP applicatif/recherche/queue selon la recette → logs/exec → deuxième déploiement → destruction/inventaire. Tester le pull runtime indépendamment du login de la CI et préserver les releases référencées.
5. Si le budget ne permet pas une partie du scénario, enregistrer précisément `NOT_RUN`. Ne pas remplacer la recherche par `disabled` puis annoncer une recette Magento entièrement testée.
6. Corriger seulement les défauts reproduits de ce chemin. Les variantes Aurora, EKS, Managed Instances, brokers alternatifs et HA restent dans leur état documenté.

### ML-07b : extraire sans réinventer

1. Réutiliser le protocole, la négociation, le cache et le packaging GCP. Pas de deuxième transport RPC ni de deuxième orchestrateur applicatif.
2. Déplacer progressivement l’exécution Pulumi AWS et les opérations nécessaires vers `providers/aws/`. Préserver les identités de stacks, le backend, les URNs et les noms ; employer import/alias/migration explicite si un changement les affecte.
3. Les schémas Magento et les étapes de migration restent communs. Les topologies réseau/base/cache/search restent AWS.
4. Extraire dans un module de support étroit uniquement les helpers réellement requis pour couper un couplage ; ne pas déplacer toute l’arborescence par principe.
5. Préserver la lecture et le nettoyage des anciens états. Le passage en plugin ne doit pas faire créer une deuxième infrastructure ou rendre d’anciennes ressources inaccessibles.
6. Ajouter une vérification de dépendances et mesurer la différence de taille. Les SDK OVH/Scaleway encore nécessaires à un ancien chemin ne justifient pas de bloquer l’alpha : documenter le reliquat, éviter d’ajouter des imports, traiter l’isolation dans une PR ciblée ultérieure.
7. Tests de parité hors cloud, puis essai réel minimal seulement dans le solde prévu. Ne pas relancer tout GCP pour un changement AWS sans modification du contrat commun.

**Fin :** AWS a un niveau de test explicite et suit le contrat partagé. L’absence de crédits pour une combinaison n’empêche pas sa publication comme expérimentale ; elle empêche uniquement de la prétendre testée.

## ML-08 — Interface machine, skills et console locale

**But :** une même couche d’opérations utilisable par les humains, les agents et une petite UI.

**Zones :** `internal/cli/`, `internal/usererr/`, ports existants, `agents/skills/`, manifeste généré des skills. Nouveaux emplacements proposés uniquement si nécessaires : `internal/console/` et ses assets embarqués.

### ML-08a : contrat machine

1. Conserver `--output json` et `--no-interaction`, sans multiplier les variantes de flags.
2. Documenter le résultat de chaque commande importante et ses codes de sortie. Ajouter le versionnement structuré de manière additive où il manque ; ne pas casser silencieusement les utilisateurs existants.
3. stdout = résultat machine ou flux explicitement documenté ; stderr = progression/diagnostic. Répercuter correctement le code retour d’un exec.
4. Erreurs identifiables, retryabilité lorsque connue, environnement/compte sélectionnés et explication de la prochaine action. Pas de copie automatique de secrets dans les détails.
5. `--no-interaction` échoue clairement lorsqu’une information/approbation manque. `--yes` ne donne pas de nouveaux droits IAM et ne sélectionne pas arbitrairement la production.
6. Ajouter à `doctor` ou à un diagnostic existant un bundle expurgé si nécessaire. L’export est local et volontaire ; aucun envoi automatique aux mainteneurs ou à un LLM.
7. Mettre les skills en accord avec les commandes réellement distribuées. Vérifier leurs exemples par des tests de l’aide/parser ; pas de commande imaginée dans une skill.
8. Garder l’IA facultative : aucun compte LLM, backend hébergé ou agent intégré requis. MCP peut être ajouté plus tard comme adaptateur, sans bloquer ce lot.

### ML-08b : console locale en lecture seule

1. Une commande proposée `magelift console` lance le serveur local ; vérifier qu’elle n’existe pas déjà avant création. Ne pas détourner `env ui`, qui peut avoir une autre fonction.
2. Choix minimal : Go HTTP/templates et JavaScript natif, assets via `go:embed`. Pas de nouvelle app Next.js ni de service distant. Pas de Node.js requis chez l’utilisateur.
3. Lire le même modèle d’état que la CLI : environnements du projet, statut, release active, date du dernier relevé, résultat du dernier déploiement et logs récents bornés.
4. Extraire seulement les fonctions de lecture nécessaires des handlers CLI. Ne pas appeler le binaire via shell à chaque rafraîchissement et ne pas introduire un deuxième état durable.
5. Ne pas charger GCP pour afficher AWS, ni faire échouer toute la page si un provider est absent. Afficher `unknown`, `unavailable` ou « non pris en charge » plutôt qu’une couleur verte inventée.
6. Limiter l’écoute à la boucle locale, protéger l’accès avec un secret de session aléatoire et valider Host/Origin. Pas de CORS permissif. Ne pas stocker le token dans les logs ou l’historique d’une URL durable.
7. Délais d’expiration/cache/pagination des logs ; fermeture des processus providers à l’arrêt ; aucun appel cloud incontrôlé à chaque frame.
8. Aucune mutation dans cette version : ni déploiement, ni delete, ni terminal DB web intégré.

### Vérification

JSON strict en réussite et échec, aucune sortie colorée parasite, secrets fictifs expurgés, non-interactif déterministe, navigation multi-env, provider manquant, données périmées visibles, refus des requêtes cross-origin non autorisées, fermeture propre.

**Fin :** la console apporte de la visibilité sans créer un second control plane. Une UI minimale peut être publiée avant la parité AWS complète.

## ML-09 — Permettre la contribution sans devenir un framework universel

**But :** rendre les extensions réalistes pour un contributeur.

**Zones :** `sdk/`, `docs/adding-a-provider.md`, `CONTRIBUTING.md`, exemples et adaptateurs existants.

### Travail

1. Documenter les quelques opérations obligatoires et les capacités optionnelles réellement issues de GCP/AWS. Une capacité non implémentée renvoie une erreur explicite ; pas de no-op vert.
2. Réutiliser ou compléter un exemple minimal de provider qui se compile et passe une suite de conformité sans compte cloud. Pas de marketplace ni d’auto-installation de code inconnu.
3. Exiger d’une contribution un propriétaire/mainteneur, un exemple YAML, des tests hors cloud et le relevé honnête des essais réels. Ne pas exiger de crédits dans tous les clouds pour proposer une PR.
4. Préserver les providers OVH/Scaleway et leurs preuves. Ils peuvent être proposés explicitement comme expérimentaux. Ne pas étendre leur promesse sans vérification ni inventer un mainteneur communautaire.
5. Intégrations : accepter d’abord des destinations et ressources externes via les contrats existants — endpoint compatible, référence de secret, OTLP, SMTP ou edge préconfiguré selon les capacités réellement présentes.
6. Distinguer référence à une ressource externe, adoption et suppression. Aucun endpoint fourni par l’utilisateur ne doit transférer implicitement la propriété de la ressource à MageLift.
7. Pas de provisionnement complet de New Relic, Datadog, Fastly, Cloudflare et tous les fournisseurs email dans ce lot. Ajouter un adaptateur demandé au moment où un contributeur ou pilote le teste.
8. Reporter le gel du SDK et les releases indépendantes jusqu’à un besoin concret. Les versions coordonnées sont acceptables pendant l’alpha.

**Fin :** un contributeur sait ajouter un provider sans modifier tout le noyau ni acheter un banc de test multi-cloud.

---

## 5. Budget et discipline cloud

### GCP

Utiliser GCP pour les tests réels de build, migrations, données, recherche, queue, médias et exploitation. Un runner GCP de build/test peut être utilisé si le poste de développement manque de mémoire, à condition d’être explicitement créé dans le projet autorisé et inventorié.

Conserver une petite recette standard ; limiter les runs concurrents, les durées et les objets retenus. Les crédits ne sont pas une autorisation de créer des ressources sans propriétaire. Distinguer consommation brute et montant net après crédits pour ne pas afficher un coût réel de zéro.

### AWS — enveloppe proposée

Les chiffres suivants sont des plafonds de planification, pas des estimations de prix AWS ni une garantie de facture.

| Usage | Enveloppe de travail |
|---|---:|
| Test groupé de la recette ECS existante | 50 USD |
| Parité après extraction du provider | 30 USD |
| Régressions ciblées | 20 USD |
| Réserve non planifiée sur les ~150 USD annoncés | 50 USD |

**Total des essais planifiés : 100 USD maximum, sous réserve de vérification du crédit restant et des dépenses en cours.** Ne pas autoriser la dépense sur carte bancaire pour compléter un test.

Avant un run AWS :

1. Vérifier compte, région, rôle et périmètre autorisés. Pas d’inférence depuis un préfixe de nom historique.
2. Lire le crédit restant lorsque l’accès le permet ; sinon demander ce seul renseignement avant la création. Vérifier expiration/couverture des crédits et prendre en compte les autres consommations du compte.
3. Consulter les tarifs actuels des seuls services utilisés et estimer durée de création, test, destruction, stockage résiduel et marge. Un prix inconnu n’est pas zéro.
4. Enregistrer une réservation indicative avant le run dans un relevé mainteneur simple, ignoré par Git s’il contient des informations de compte. Éviter un nouveau système FinOps.
5. Une seule stack AWS de test active par défaut. Aucun run automatique sur toutes les PRs ; lancement manuel sur commit de confiance.
6. Si le nouveau run ne tient pas dans l’enveloppe disponible, ne pas créer. Continuer les tests locaux et noter le blocage.
7. Timeout et nettoyage de sortie, plus un balayage de récupération indépendant pour les runs interrompus. Les erreurs de nettoyage sont visibles, jamais masquées par un résultat applicatif vert.
8. Arrêter les nouveaux essais lorsque la consommation engagée et estimée atteint l’enveloppe de 100 USD ; préserver la réserve pour les délais de facturation et résidus. Ne pas « attendre l’alerte » pour arrêter.

AWS Budgets peut notifier après le dépassement. Les budgets GCP de type « alerts-only » ne plafonnent pas non plus automatiquement les dépenses. [S11, S12] Ces garde-fous réduisent le risque sans promettre une coupure exacte à 100 ou 150 USD. Nettoyer les ressources déjà engagées peut encore générer des frais pendant leur suppression.

## 6. Politique de test et de release

### À chaque PR

Tests des packages affectés, régression du bug corrigé, génération/schema/docs quand concernés. Aucun secret cloud pour le code de forks. Pas de workflow `pull_request_target` qui exécute du code de contribution non revu avec des credentials.

### Avant merge d’un lot

`make verify-community` — **cible à créer/réconcilier dans ML-00** — couvre les modules concernés et le parcours de référence hors cloud. Les tests plus larges restent disponibles et leurs exclusions sont explicites.

Les tests doivent échouer sur un défaut réel. Ne pas remplacer un test applicatif rouge par un mock vert, retirer une assertion ou transformer une erreur en `SKIP` pour clôturer le lot.

### Essais cloud

GCP sur changement significatif du parcours GCP ou des contrats communs. AWS sur changements AWS suffisamment regroupés, dans l’enveloppe. Pas de produit cartésien versions × clouds × runtimes × services.

Un rapport court suffit : SHA, versions/digests, configuration, étapes effectuées, résultat et restes. Réutiliser l’outillage de preuve existant lorsqu’il fonctionne ; ne pas développer un nouveau système de scellés pour publier une alpha.

### Publication

Les vérifications de sécurité de l’artefact distribué restent utiles. En revanche, ni la conformité de tous les providers ni la résolution de toutes les issues n’est exigée.

Les bugs non bloquants sont documentés. Les capacités non testées sont identifiées. Les tags, la publication publique et les actions destructrices sur environnements protégés restent soumis à l’approbation du mainteneur.

## 7. Règles d’exécution pour Codex

1. Commencer par ML-00. À chaque session suivante, reprendre le premier lot non terminé et débloqué, pas un nouvel audit général.
2. Lire `AGENTS.md`, la roadmap et uniquement les skills/fichiers nécessaires au lot. Les documents historiques ne doivent pas remettre en file active des travaux gelés.
3. Avant création, chercher l’équivalent dans le dépôt : types, erreur, méthode, commande, manifeste, scheduler, fichier de preuve.
4. Annoncer une liste courte de fichiers et tests ; implémenter. Pas de longue phase de rédaction au lieu du code.
5. Réutiliser les contrats. Ne pas ajouter un registre, une couche d’indirection ou un DSL pour une seule implémentation.
6. Ne pas profiter d’une tâche pour mettre toutes les dépendances à jour, renommer tous les packages ou reformater le dépôt.
7. Ne pas modifier la sémantique publique d’une commande sans justification, migration et test. Les nouveaux chemins/commandes de ce plan sont des propositions d’implémentation, pas l’affirmation qu’ils existent déjà.
8. Maintenir les tests de régression et les garde-fous destructifs. Une feature flag n’autorise pas une suppression dangereuse.
9. Une impossibilité d’exécuter un test est notée `NOT_RUN` avec sa cause. Le reste du lot peut avancer ; ne pas inventer la validation.
10. Ne pas faire dépendre une tâche locale d’une approbation cloud inutile. Mais ne pas déployer dans un compte non autorisé pour éviter une question nécessaire.
11. Pas de tag, publication ou réécriture d’historique implicite. Pas d’accès aux données d’une vraie boutique pour fabriquer une fixture.
12. Un commit/une PR doit résoudre un résultat utilisateur et inclure le test pertinent. Pas de commit « tout terminé » fondé sur une mise à jour de checklist.

### Compte rendu obligatoire en fin de lot

```text
Lot : ML-XX
Base / commit : ...
Livré : ...
Fichiers principaux : ...
Tests exécutés : commandes exactes + résultat
Essais cloud : compte/projet expurgé + opérations + résultat, ou NOT_RUN
Budget AWS : estimation réservée / consommation connue / incertitude restante
Ressources restantes : aucune vérifiée, retenues explicitement, ou inventaire précis
Limites et bugs connus : ...
Documentation modifiée : ...
Prochain lot débloqué : ...
```

Pas de valeurs de secrets, URL présignée, dump, tokens ni identifiants privés inutiles dans ce rapport.

## 8. Prompt de démarrage à transmettre avec ce document

```text
Implémente MageLift à partir du plan CODEX_IMPLEMENTATION_PLAN.md.

Le plan est la directive produit du mainteneur : continuité du dépôt,
GCP comme terrain principal, AWS limité aux crédits disponibles, alpha
communautaire sans certification exhaustive. Conserve Go, Pulumi, YAML,
le build PHP, le manifeste et le contrat provider existants.

Applique la révision 1.1 : aucune image Docker MageLift centrale à publier.
Distribue les recettes versionnées ; construis builder/runtime/application
sur la CI du projet utilisateur et pousse vers son registre choisi.
Conserve BuildKit, les caches utilisateur, le manifeste existant et la
promotion par digest sans reconstruction. Le workflow CI minimal est une
condition du jalon A, pas un chantier post-alpha.

Commence par ML-00, puis enchaîne les lots débloqués dans l’ordre.
Ne relance pas un audit général et ne recrée pas les éléments déjà livrés.
Réconcilie AGENTS.md et intent/ROADMAP.md dans ML-00 pour ne pas rester
bloqué sur les anciennes obligations de certification ou de replanification.

Travaille sur reboot/community-alpha depuis wip/all-local-work sans écraser
le travail local. Pour chaque lot, fais des modifications ciblées, ajoute
les tests, exécute-les et fournis le compte rendu demandé dans le plan.

Pas de nouvelles abstractions sans nécessité démontrée. Pas de nouveau
manifeste, framework de plugins, moteur de certification, chatbot ou
migration de langage. Les providers expérimentaux restent disponibles
avec leur niveau réel de test ; ils ne bloquent pas la première alpha.

Aucun déploiement hors cible d’acceptance explicitement autorisée. Aucun
secret committé. Aucun tag/publication ni destruction protégée sans
approbation. Si un test cloud est impossible, note NOT_RUN et avance sur
les tâches locales indépendantes, sans simuler une réussite.

Arrête-toi avant une action nécessitant une autorisation manquante,
pas devant une tâche de code déjà couverte par ce plan.
```

## 9. Sources et points de contrôle

Les références de dépôt ci-dessous sont relatives à `magelift/magelift` au SHA inspecté en tête de document. Elles servent à localiser le travail existant, pas à imposer un comportement historique contre le code courant.

- **[S1]** `intent/ROADMAP.md` — GCP alpha, AWS ensuite, recette et périmètre existants.
- **[S2]** `intent/alpha-development-loop/plan.md` — tâches déjà réalisées, tests et vérifications restantes.
- **[S3]** `intent/transparent-provider-delivery/intent.md` ; `internal/providerhost/` — acquisition/résolution/cache.
- **[S4]** `internal/registry/registry.go` ; `docs/adr/0013-provider-plugin-contract.md` — frontière provider et transition en cours.
- **[S5]** `intent/published-build-images/intent.md` (objectif de publication centrale remplacé par la révision 1.1) ; `build/src/Artifact/ArtifactManifest.php` ; `sdk/artifact.go` — contrats existants à préserver.
- **[S6]** `build/src/Magento/PatchLifecycle.php`, `PatchApplier.php`, `LifecyclePlan.php` — build et patches.
- **[S7]** `agents/README.md` ; `agents/skills/` — skills distribuées.
- **[S8]** `internal/cli/root.go`, `internal/cli/env_ui_test.go` — CLI et sortie machine.
- **[S9]** `AGENTS.md` ; `Makefile` — obligations de travail et contradiction de la cible race.
- **[S10]** Adobe, Quality Patches Tool, Installation and usage : `https://experienceleague.adobe.com/en/docs/commerce-operations/tools/quality-patches-tool/usage` ; dépôts `magento/quality-patches` et `magento/magento-cloud-patches`. Lire les versions effectivement verrouillées avant implémentation.
- **[S11]** AWS, Managing your costs with AWS Budgets : `https://docs.aws.amazon.com/cost-management/latest/userguide/budgets-managing-costs.html` — délai de facturation et de notification.
- **[S12]** Google Cloud, Create, edit, or delete budgets and budget alerts : `https://docs.cloud.google.com/billing/docs/how-to/budgets` — distinction alertes seules et autres dispositifs de plafond selon service.

### Références ajoutées pour la révision 1.1

- **[S13]** Docker, Registry cache : `https://docs.docker.com/build/cache/backends/registry/` — cache indépendant de l’image finale et stages intermédiaires.
- **[S14]** Docker, Build secrets : `https://docs.docker.com/build/building/secrets/` — montages de secrets, éviter ARG/ENV persistants.
- **[S15]** Docker, Building best practices : `https://docs.docker.com/build/building/best-practices/` — bases, multi-stage, pinning et mises à jour.
- **[S16]** Kubernetes, Pull an Image from a Private Registry : `https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/` — authentification de pull runtime.
- **[S17]** AWS, Using non-AWS container images in Amazon ECS : `https://docs.aws.amazon.com/AmazonECS/latest/developerguide/private-auth.html` — `repositoryCredentials`, Secrets Manager et rôle d’exécution.
- **[S18]** Google Cloud, Deploying to Google Kubernetes Engine : `https://docs.cloud.google.com/artifact-registry/docs/integrate-gke` — permissions du pull depuis Artifact Registry.
- **[S19]** `internal/cli/build.go`, branche `wip/all-local-work`, blob `44b13597e6eab82f4ae14b462e822d5475eca125` — prérequis `--push`, étapes et signatures déjà présents lors de la relecture.
- **[S20]** Docker, Optimize cache usage : `https://docs.docker.com/build/cache/optimize/` — invalidation, contexte et cache mounts.

**Règle finale : publier une capacité limitée qui fonctionne, documenter les défauts réels, puis itérer. Ne pas transformer ce plan en un nouveau projet de plateforme universelle.**
