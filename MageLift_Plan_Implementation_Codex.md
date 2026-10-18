# MageLift. Plan Codex pour un mainteneur solo et des agences Magento

**Superseded as the active queue by `IMPLEMENTATION_PLAN.md` (v1.3).**
This file stays as the v1.2 record. Do not execute its checklists in parallel.

**Version 1.2, 18 octobre 2026**  
**Remplace intégralement les versions 1.0 et 1.1 ; ce n’est pas une liste d’exigences supplémentaire.**  
**Dépôt :** `magelift/magelift`  
**Base :** `wip/all-local-work`. Dernier SHA identifié dans l’analyse précédente : `3ac1b844a7117752f3a5045054587508c16dcf6c`. Relever le HEAD réel avant de travailler.  
**Branche :** réutiliser `reboot/community-alpha` si elle existe déjà ; sinon la créer depuis la branche de travail, sans écraser les modifications locales.

## 1. Décision produit

MageLift doit permettre à un développeur Magento d’une petite agence de construire, déployer et exploiter une boutique dans le compte cloud de son client, sans concevoir lui-même une infrastructure ni maintenir des Dockerfiles de framework.

Le mainteneur fournit le logiciel et ses recettes. Il ne fournit ni hébergement, ni astreinte, ni fabrique centrale d’images. Les défauts sont acceptables dans une alpha ; les fausses réussites, les suppressions hors périmètre et les promesses opérationnelles trompeuses ne le sont pas.

**Le critère de simplicité n’est pas le nombre de commandes : c’est le nombre de décisions techniques et de manipulations manuelles imposées à l’utilisateur, puis au mainteneur pour l’aider.**

### Contraintes conservées

- Continuer Go/Cobra, Pulumi, YAML, PHP et les contrats existants. Aucune réécriture générale, aucun nouveau moteur d’infrastructure ou de plugins.
- Aucune image Docker spécifique à MageLift publiée centralement, sur GHCR ou ailleurs. Les recettes et scripts sont versionnés avec le logiciel ; les images sont construites dans la CI utilisateur et poussées dans son registre.
- Les images amont non modifiées restent utilisables. Les helpers spécifiques à MageLift doivent eux aussi être construits chez l’utilisateur lorsqu’ils sont nécessaires.
- GCP est le terrain principal : crédits annoncés illimités. AWS conserve un parcours expérimental et reçoit des essais ciblés : environ 150 USD de crédits annoncés.
- La première intégration CI documentée et exécutée est GitHub Actions. La CLI reste indépendante de GitHub pour son exécution.
- Le registre est configurable. Artifact Registry est le défaut proposé sur GCP ; ECR sur le futur parcours AWS. GHCR utilisateur et les autres registres ne sont pas interdits, mais leurs chemins d’authentification ne sont pas tous une condition de première publication.
- Les providers et capacités expérimentales existants restent récupérables et honnêtement présentés. Ne pas supprimer un chemin nécessaire pour exploiter ou nettoyer une ancienne stack.

### Corrections de priorité par rapport à v1.1

| Sujet | Décision v1.2 |
|---|---|
| « Compte et CI correctement préparés » | La préparation guidée fait partie du produit. Facturation, droits accordés par un administrateur et propriété du domaine restent des prérequis externes explicites. |
| Diagnostic, JSON et skills tardifs | Le minimum utile appartient à la première alpha, pour réduire les demandes de support et permettre l’automatisation. |
| Extraction AWS planifiée comme passage obligé | Parcours AWS utile d’abord. Extraction seulement si elle résout un problème mesuré ou débloque une contribution réelle. |
| Plusieurs voies de distribution à qualifier ensemble | Une même release produit les archives et métadonnées Brew/Scoop. Distinguer installation réussie et capacité runtime réellement testée. |
| Générateur CI autoritaire | Le workflow appartient à l’agence. Pas de remplacement automatique de ses personnalisations. |
| Publication puis exploitation ambiguë | Alpha sur environnement de test d’abord. Le passage d’une boutique réelle exige une procédure minimale de restauration et de migration validée sur cette boutique. |
| Longue roadmap technique obligatoire | Quatre lots vers l’alpha ; deux lots suivants selon les pilotes ; autres idées dans un backlog non actif. |

La relecture couvre le plan v1.1 complet et des chemins ciblés de code/documentation. Aucun build, test automatisé, essai utilisateur ou déploiement cloud n’a été exécuté pour cette révision. Les comportements décrits ci-dessous sont des objectifs ; les noms de flags nouveaux doivent être confirmés contre l’aide réelle.

## 2. Le parcours que l’utilisateur doit obtenir

### 2.1 Un développeur Magento, pas un administrateur cloud

Le premier écran d’initialisation doit demander des informations que l’utilisateur connaît : son projet Magento, son compte/projet cloud, sa région, le dépôt Git, le domaine de test et éventuellement son registre existant. Proposer un défaut pour le registre, la recette et les tailles. Ne pas faire choisir d’emblée un type de cluster, un mécanisme de réseau privé, une topologie HA, un issuer de signature ou un backend Pulumi.

L’initialisation inspecte les fichiers déjà présents, notamment `composer.json`, `composer.lock` et `app/etc/config.php`. Elle propose les versions, locales et paramètres qu’elle peut déterminer sans ambiguïté, puis demande confirmation pour le reste. Elle ne migre ni Magento ni les données, n’active pas tous les modules et ne remplace pas les réglages applicatifs existants.

Une version ou extension non couverte doit produire une limite précise avant de créer des ressources. Un simple scan ne peut pas prouver la compatibilité de toutes les extensions : le rapport doit le dire.

### 2.2 Séparer préparation initiale et utilisation quotidienne

| Moment | Résultat attendu | Ce que l’utilisateur ne doit pas assembler lui-même |
|---|---|---|
| Installation | Binaire disponible, version affichée, aide accessible | Compilation Go, installation manuelle d’un provider à côté du binaire |
| Initialisation | YAML court, défauts visibles, fichiers à committer identifiés | Schémas Pulumi, versions internes, noms de ressources cloud |
| Préparation initiale | Identité, APIs, registre, état et permissions de CI préparés ou manque explicitement identifié | Longue liste de rôles IAM et de commandes de fédération |
| Premier push | CI construit, publie, déploie et fournit URL, digest et résultat | Recopie de scripts Docker et manipulation du manifeste |
| Exploitation | État, logs, commande Magento et informations de connexion contrôlées | Recherche d’un pod ou d’un ARN à chaque incident |
| Incident | Étape fautive, cause connue, résultat conservé, prochaine action sûre | Reconstitution manuelle de toutes les sorties de CI |
| Mise à jour | Version proposée, diff, test hors production, promotion | Découverte après coup que la mise à jour CLI a changé la base de données |

Réutiliser `init`, `bootstrap`, `doctor`, `ci generate`, `build`, `preview`, `deploy` et les commandes d’exploitation existantes. `preview` conserve son sens de plan d’infrastructure. Ne pas introduire une famille de commandes concurrente pour rendre les exemples plus élégants.

### 2.3 Le bootstrap est une vraie fonctionnalité

Compléter le bootstrap existant, pas créer un service d’identité MageLift. Une identité autorisée par le propriétaire du compte exécute une préparation ponctuelle, annoncée et réexécutable. Elle peut utiliser le login cloud officiel ; MageLift ne doit pas réimplémenter OAuth.

La recette prépare les APIs, identités, permissions bornées, fédération GitHub et accès au registre dont elle a besoin, sous réserve des droits et politiques de l’organisation. Le plan doit expliquer les changements avant leur application. Si les permissions manquent, produire une demande concise et ciblée pour l’administrateur, plutôt qu’exiger un rôle Owner permanent ou un secret de compte de service durable.

La création du registre, de la fédération et du stockage d’état précède le build qui les utilise. Réutiliser le backend, la gestion de clés et les verrous existants. Ne pas introduire une autre base d’état. Les ressources partagées de bootstrap ont une durée de vie distincte de celle d’un environnement.

Les instructions DNS doivent nommer exactement les enregistrements attendus et leur état. Une attente de DNS/TLS n’est pas une raison de recréer l’infrastructure. Ne pas commencer par implémenter les APIs de tous les registrars.

### 2.4 Un projet, une configuration compréhensible

Le YAML contient l’intention ; les fichiers générés contiennent les détails. Réutiliser les locks et le manifeste déjà présents. Ne pas ajouter plusieurs fichiers de versions à maintenir à la main ni fusionner tous les formats existants dans un nouveau format universel.

Le starter ne déploie qu’un environnement de test. Une production, la HA et des previews à chaque PR sont des choix explicites ultérieurs. Ne pas faire hériter à une production les dates d’expiration, petits quotas, secrets ou politiques de destruction d’une preview.

Les défauts doivent être inspectables dans la configuration effective. Les options avancées restent possibles là où elles existent, mais ne surchargent ni l’assistant initial ni le guide de démarrage.

## 3. Limites de complexité pour le mainteneur

| Surface | Règle de départ |
|---|---|
| Dépôt et architecture | Monorepo existant, provider GCP séparé conservé, AWS embarqué tant que cela ne bloque pas un objectif concret. |
| Release | Une version produit choisie par l’utilisateur. Les modules internes peuvent avoir leurs tags techniques existants ; leur coordination est automatisée, pas exposée au client. |
| Build | Une recette de référence, une architecture CPU de référence, une distribution Linux conservée. Pas de migration Alpine/Debian opportuniste. |
| Configuration | Modèle commun existant et paramètres propres à chaque provider ; pas de langage de description supplémentaire. |
| CI | Une intégration GitHub Actions mince. La logique Magento reste dans le package PHP ; la logique cloud dans le provider. |
| Documentation | Un démarrage canonique, une référence générée quand elle existe, un guide de dépannage. Ne pas entretenir plusieurs parcours contradictoires. |
| Tests | Tests locaux du code concerné ; une recette GCP réelle ; AWS à la demande. Pas de nouvelle infrastructure de certification. |
| Support | Issues/PRs et canal privé sécurité existants. Pas de SaaS de support, ni collecte automatique de dumps ou de logs. |

Un fichier long, un binaire plus gros que souhaité ou une interface interne imparfaite ne déclenchent pas, seuls, une réécriture. Mesurer le problème : lenteur de compilation, impossibilité de contribuer, taille de téléchargement, défaut de distribution ou couplage qui empêche une correction.

Inversement, ne pas laisser durablement une frontière en cours de migration multiplier les modes d’exécution. Toute extraction retenue doit retirer l’ancien chemin à une échéance de code définie, tout en préservant l’accès aux anciens états. Ne pas ajouter une troisième architecture transitoire.

## 4. Lots exécutables

Les nouveaux IDs S0-S5 remplacent les anciens ML-xx dans la roadmap active. Les tâches déjà implémentées restent acquises après vérification ; ne pas les refaire. S0-S3 conduisent à l’alpha. S4 et S5 sont ensuite priorisés selon les pilotes et peuvent avancer indépendamment.

### S0: Une boucle de correction utilisable

**Zones :** `AGENTS.md`, `intent/ROADMAP.md`, `Makefile`, CI et tests des modules concernés, scripts de développement existants.

1. Relever branche, HEAD et changements locaux. Réconcilier la roadmap et les règles avec ce plan, sans archiver une tâche inachevée comme terminée. Réutiliser la branche de reprise si elle existe.
2. Corriger la contradiction entre les instructions et la cible `make test`, qui lance un race global. Fournir une commande locale raisonnable et une vérification avant merge couvrant explicitement racine, SDK et provider GCP. Les tests de modules imbriqués ne doivent pas être oubliés. Garder le race ciblé et une voie CI adaptée aux vérifications plus lourdes.
3. Pouvoir exécuter les tests PHP sans exiger le bon PHP sur le poste : construire le conteneur de test localement depuis une base amont, avec les recettes existantes. Rien à publier centralement.
4. Rejouer les régressions déjà présentes sur fermeture des providers et ordre des migrations. Ne pas rouvrir un ancien défaut corrigé parce qu’un document le mentionne encore.
5. Réutiliser les artefacts de développement adressés par commit. Une correction locale ne dépend pas d’un tag public ni d’un scellé documentaire. Les vérifications de confiance des artefacts distribués restent actives.
6. Préparer une fixture Magento synthétique avec un module et un thème de test simples, pour ne pas valider seulement une installation vierge. Aucun code ou dump d’un employeur n’est copié.

**Sortie :** un contributeur peut modifier le code concerné et tester sa correction sans compte cloud. Une session reprend le travail restant au lieu de recommencer un audit global.

### S1: Construire avec le logiciel distribué, pas avec ton workspace

**Zones :** `images/`, `build/`, `internal/build/`, `internal/toolchain/`, `internal/cli/build.go`, manifeste existant, distribution CLI/providers.

1. Livrer les sources de recette et le PHP nécessaire avec la version de MageLift, de préférence via l’embarquement déjà disponible ou `go:embed`. Pas de checkout du framework par l’utilisateur, de résolution de branche `main` au build ni d’images Docker MageLift centrales, y compris pour les helpers requis.
2. Retirer l’obligation de fournir des images builder/runtime préconstruites à `build --push`. Construire sur le runner avec les Dockerfiles ou intermédiaires du pipeline existant. Garder les overrides avancées fonctionnelles ; ne pas créer deux moteurs.
3. Préserver un seul lifecycle PHP : Composer, patches, DI, contenu statique. Tester QPT et hotfixes sur la fixture. Les cloud patches restent une option bornée : délégation à l’outil upstream verrouillé, périmètre affiché, absence de double application. Pas de catalogue parallèle, sélection IA ni application générale par défaut hors Adobe Cloud.
4. Pinning des bases/outils, secrets BuildKit, exclusion des dumps et `env.php` runtime, cache utilisateur privé lorsque nécessaire. Le build froid doit fonctionner ; le cache registry est une optimisation. Ne pas promettre de reproductibilité bit à bit à partir des seuls digests de base.
5. Construire depuis le checkout privé du projet ; un HEAD détaché normal en CI doit être accepté avec un commit identifié. Ne pas exiger une branche locale, une identité Git d’auteur ou une URL de dépôt publique pour construire. Le mode release exige des entrées enregistrées ; le développement local peut signaler un arbre modifié au lieu de le présenter comme une release reproductible.
6. Une seule plateforme Linux/amd64 pour le chemin GCP initial. Publier les images déployables dans le registre configuré, finaliser le manifeste existant avec leur digest, puis utiliser ce manifeste pour déployer. La promotion ne reconstruit pas.
7. Réutiliser l’acquisition vérifiée des providers et outils. Pas de Go, PHP ou Node à installer chez l’utilisateur final. Docker/Buildx n’est requis que sur la machine qui construit ou exécute le développement local, pas pour toutes les commandes de lecture. Le moteur Pulumi sous-jacent reste géré et visible dans le diagnostic technique.

**Tests déterminants :** build froid depuis un artefact distribué et une boutique synthétique ; cache chaud puis patch modifié ; secrets fictifs absents des résultats ; signature invalide refusée ; promotion sans appel au builder ; refus clair d’un composant indispensable absent.

**Sortie :** la recette est utilisable sans accès à ton workspace, sans construire une image centrale et sans manipuler les détails de packaging. Un défaut périphérique de registre ne bloque pas le chemin de référence.

### S2: Préparer la petite agence et sa CI

**Zones :** initialisation existante, `internal/cli/bootstrap.go`, bootstrap GCP, `internal/cli/ci.go`, configuration, onboarding.

1. L’initialisation inspecte la boutique et génère un YAML minimal pour un environnement de test. Détecter avant création les incompatibilités connues et les fichiers requis manquants ; ne pas proposer toutes les topologies.
2. Compléter le bootstrap guidé décrit en section 2.3. Une relance ne recrée pas les mêmes objets et ne prend pas possession de ressources étrangères. Générer/récupérer les identifiants nécessaires à la CI ; ne pas les faire reconstituer dans la console cloud.
3. Préparer un workflow utilisateur GitHub Actions : checkout, installation épinglée, authentification, Buildx, build/push, conservation du manifeste, déploiement autorisé. Paramètres ordinaires du projet dans le YAML ; références d’identité non secrètes dans la sortie bootstrap/configuration appropriée ; secrets dans le stockage prévu. Pas de copie de logique Magento dans le YAML CI.
4. **Respecter le workflow utilisateur.** `ci generate` crée un fichier absent ; s’il diffère, proposer une sortie temporaire/diff et un remplacement explicite. Ne pas écrire un moteur de fusion YAML. L’égalité octet à octet au template devient un contrôle de fraîcheur optionnel, pas une validation de toute CI personnalisée. Ne pas ajouter une validation sémantique universelle des workflows.
5. Utiliser la fédération d’identité disponible, limitée au dépôt et aux références/environnements autorisés. Séparer les droits de bootstrap, build, déploiement et pull runtime selon la recette. Les contributions non approuvées ne reçoivent ni secrets, ni cache privé, ni privilèges cloud. Une inspection du build ne permet pas de sandboxer un script Composer malveillant : le code exécuté avec des secrets doit être de confiance.
6. Chemin initial : même cloud et registre utilisateur natif. Le code accepte les destinations externes, mais le guide ne promet que les combinaisons effectivement câblées. Une CI pouvant pousser ne prouve pas que le runtime peut télécharger après expiration de ses credentials. Ne pas implémenter une réplication inter-registres générique pour fermer l’alpha.
7. Ne pas masquer coût, durée attendue des opérations cloud ou exigences machine. Publier une configuration de runner réellement testée et la consommation brute observée des services. Si le runner CI standard ne suffit pas, documenter une alternative utilisateur précise ; aucune hypothèse de gratuité fondée sur tes crédits.

**Tests déterminants :** bootstrap répété ; permission/API manquante avec action lisible ; workflow personnalisé non écrasé ; HEAD détaché ; build et déploiement depuis un autre dépôt autorisé ; pull runtime indépendant du login CI ; PR non autorisée sans privilèges.

**Sortie :** on ne renvoie plus une agence sans DevOps à une longue liste de manipulations IAM. Ce qui nécessite réellement son administrateur ou registrar est identifié séparément et une seule fois.

### S3: Livrer une alpha exploitable et supportable

**Zones :** opérations et déploiement existants, `doctor`, logs/health/exec, `internal/usererr/`, skills, docs, release.

1. Exécuter le parcours S1-S2 sur la recette GCP existante : GKE Autopilot, nginx/PHP-FPM, Cloud SQL, Valkey, OpenSearch et médias GCS privés. Vérifier la combinaison Magento/services contre les exigences applicables au moment de l’essai. La queue DB reste le choix alpha, avec ses limites ; ne pas lancer une migration de broker générale.
2. Vérifier HTTP applicatif, asset, création/lecture de donnée synthétique, recherche réellement indexée, cron, queue et médias autorisés/interdits. Déployer une deuxième release avec un changement observable et vérifier la persistance des données. Les migrations sont contrôlées une fois ; elles ne tournent pas à chaque démarrage de réplique.
3. Traiter un incident utile : configuration erronée, credential expiré ou migration échouée. Indiquer projet, environnement, étape fautive, résultat connu et prochaine action. Une relance ne doit pas lancer simultanément deux migrations. Pas de retry aveugle d’une opération non démontrée idempotente ; ne pas supprimer l’état pour débloquer l’outil.
4. Livrer dès l’alpha les sorties JSON et non interactives des commandes importantes, plus des skills exactes. Ne pas imposer MCP ni compte LLM. `doctor` doit être utilisable avant que l’installation ou le déploiement ne réussisse ; un contrôle local ne nécessite pas de credentials cloud.
5. Ajouter au diagnostic un export volontaire et local des métadonnées utiles, par liste de champs autorisés. Aucun upload automatique. Les logs bruts sont optionnels et à relire ; l’expurgation ne garantit pas de détecter toutes les données d’une extension. Ne pas joindre de dump, token, URL présignée ou configuration secrète.
6. Une seule commande de release approuvée s’appuie sur les workflows existants pour générer archives signées, checksums, notes et métadonnées Brew/Scoop. Pas de recompilation indépendante pour chaque canal. Tester macOS et Linux annoncés ; sous Windows distinguer binaire/commandes testés et parcours WSL. Ne pas attendre toutes les combinaisons OS/runtime pour publier un canal fonctionnel.
7. Tester la destruction sur ressources jetables avec inventaire final. Garder l’état jusqu’à résolution des ressources restantes. Un uninstall local ne détruit rien dans le cloud. Une destruction d’environnement conserve les ressources partagées, releases encore utilisées et sauvegardes à retenir.
8. Une personne externe au projet suit le guide sans instruction orale cachée, sur un environnement de test. Si ce test humain n’a pas encore été possible, le noter : la simplicité d’usage n’est pas validée par les seuls tests automatisés.

**Sortie alpha :** installation, initialisation, bootstrap, CI, déploiement fonctionnel, deuxième release, diagnostic d’échec et nettoyage sont démontrés sur une recette. Publier avec ses limites, sans attendre S4/S5, une console ou la certification des autres providers. Les bugs non bloquants restent publics ; ne pas recommander une migration de production sur cette seule preuve.

### S4: Accompagner une boutique existante avant d’élargir l’exploitation

Ce lot se déclenche avec un premier pilote agence. Son objectif n’est pas la HA universelle.

- Réutiliser dump/import et media sync pour copier un jeu contrôlé dans un environnement isolé ; ne jamais commencer par adopter ou supprimer la production du client. Un import peut exécuter des données ou configurations sensibles : accès et intégrations doivent rester bornés.
- Valider une sauvegarde et restauration minimale sur la recette du pilote : données, médias, références de secrets et clé de chiffrement utile. Cette preuve conditionne la recommandation d’utiliser MageLift pour des données commerciales réelles, pas la publication de l’alpha de test.
- Expliquer le rollback applicatif et sa limite face à une migration de schéma. Si la récupération n’est pas disponible, le dire et empêcher une fausse action de rollback. Pas de restauration automatique destructive.
- Permettre de reprendre les opérations depuis un deuxième poste/runner autorisé, sans dépendre d’un état ou d’un journal détenu uniquement par le premier laptop.
- Activer ensuite les previews demandées : données synthétiques par défaut, intégrations externes neutralisées, création plafonnée, expiration avec nettoyage par CI/scheduler existant même lorsque le poste est éteint. Pas de nouvelle plateforme multi-tenant pour économiser avant mesure.
- Réutiliser un environnement local existant dans l’agence. Améliorer `magelift local` lorsqu’il est choisi ; ne pas imposer le remplacement de son outil de développement pour déployer avec MageLift.

**Sortie :** un pilote peut essayer sa boutique sans risquer la production source, vérifier sa restauration et identifier qui prend en charge les incidents. Une réussite sur une boutique ne prouve pas toutes les extensions Magento.

### S5: Rendre AWS utile sans en faire un second projet

Réutiliser les tests et l’architecture AWS ECS existants. Sur une seule recette, dans l’enveloppe autorisée, vérifier build/push ECR utilisateur, pull runtime, déploiement, quelques opérations, deuxième release et nettoyage. GCP ne sert pas de substitut à ces essais. Conserver des statuts expérimentaux pour ce qui n’a pas été essayé.

**Ne pas subordonner ce parcours à l’extraction du provider AWS.** Une fois les difficultés mesurées, une PR séparée peut appliquer le contrat GCP existant si elle réduit réellement la taille distribuée, le temps de compilation ou le couplage. Aucun autre transport RPC. Préserver identités de stacks, backend, URNs, noms et capacité de récupération ; vérifier qu’un refactoring n’implique pas de remplacement destructeur.

Un pilote européen engagé peut faire passer OVH ou Scaleway avant cette extraction. Ne pas exiger une architecture de plugins parfaite pour accepter une correction ou un test communautaire utile.

**Sortie :** un parcours AWS honnêtement documenté et une dépense contrôlée ; les investissements d’architecture restent proportionnés à un besoin réel.

## 5. Maintenance, mises à jour et pérennité

### Une version produit, pas une combinaison à assembler

Le projet et sa CI utilisent une version MageLift précise. Réutiliser les métadonnées et locks existants pour fixer recettes, bases et providers. Une version incompatible doit être expliquée avant mutation ; ne pas installer silencieusement une autre version pour la contourner.

La mise à jour de la CLI seule ne lance pas de mise à niveau de Magento, de base de données, de broker ou de cluster. Les changements de recette sont visibles et testés sur un environnement de test. Un digest déjà construit reste une référence de déploiement, pas une recette à reconstruire lors de la promotion.

Maintenir d’abord la ligne alpha courante, sans promettre plusieurs branches LTS. Garder les anciennes releases consultables ; annoncer les changements incompatibles et leur migration. Avant de publier une évolution de configuration/état, tester le chemin depuis la version précédente sur une fixture représentative. Ne pas supprimer les archives, recettes ou preuves dont des releases existantes ont encore besoin.

### Les recettes doivent recevoir les correctifs, même sans images centrales

Regrouper les mises à jour ordinaires des dépendances dans le mécanisme existant. Une mise à jour urgente de sécurité peut passer séparément. La CI MageLift construit et teste les recettes ; la CI utilisateur reconstruit ensuite ses propres images sur une mise à jour choisie. Aucun rebuild magique des boutiques par le mainteneur.

Conserver les signatures, la provenance et les SBOM déjà intégrés sur le chemin annoncé. Le workflow généré prépare l’identité de signature utilisateur ; l’agence ne doit pas concevoir son propre système de PKI. Une difficulté d’outillage ne se résout pas en désactivant silencieusement la vérification.

### Support soutenable

Une issue doit demander version, recette, résultat attendu et reproduction ; le diagnostic remplit les informations connues. Pas de délai public chiffré de réponse ou de résolution ajouté sans engagement explicite du mainteneur. Afficher une prise en compte rapide selon disponibilité, priorité aux problèmes de sécurité, données et déploiement, sans astreinte ni SLA cloud implicites.

Une PR de bug n’a pas à financer des tests multi-cloud ni à fournir un mainteneur permanent. Une nouvelle intégration importante doit avoir un périmètre, des tests et une perspective réaliste de maintenance ; le mainteneur peut la laisser expérimentale ou refuser d’en assumer le support. Ne pas inventer un propriétaire communautaire.

## 6. Tests et ressources : assez pour progresser, pas un programme de certification

| Situation | Vérification |
|---|---|
| PR ordinaire | Packages modifiés et régression ; build/contrats/génération quand concernés. Aucun secret pour un fork. |
| Changement commun ou recette GCP | Parcours de référence GCP adapté au changement, sur commit de confiance. |
| Changement AWS | Tests locaux puis essai groupé autorisé lorsque nécessaire et finançable. |
| Release | Artefacts distribués, installation annoncée, régression de référence et notes honnêtes. Pas toutes les versions × clouds × registries × OS. |
| Validation ergonomique | Guide exécuté par une personne qui ne connaît pas le code, avec les interventions manuelles consignées. |

Un compte rendu court suffit : commit, entrée de test, commandes, résultat, opérations non exécutées et restes. Préserver les preuves historiques, mais aucun nouveau moteur de certification ni scellé supplémentaire pour fermer un lot.

**GCP :** un projet d’acceptance autorisé ; un environnement de debug peut être retenu explicitement avec expiration. Les crédits du mainteneur ne sont ni ceux des clients ni une preuve de coût nul. Mesurer les coûts bruts et le besoin de ressources du runner ; ne pas améliorer l’UX en cachant le prix du profil proposé.

**AWS :** environ 150 USD annoncés, dont 50 USD conservés en réserve. Au plus 100 USD de dépenses de test planifiées après vérification du solde, de sa couverture et des autres consommations. Il ne s’agit ni d’un tarif ni d’un plafond technique garanti. Une seule stack de test active par défaut ; budget de session, durée, inventaire et nettoyage avant nouvelle création. Pas d’autorisation de paiement sur carte pour finir un test. Nettoyer même lorsqu’un essai est interrompu ; ne pas attendre une alerte budgétaire pour agir.

Sans cible autorisée, permissions ou enveloppe disponibles : avancer sur le code et noter les essais cloud `NOT_RUN`. Ne pas simuler une réussite et ne pas bloquer des tâches locales indépendantes.

## 7. Backlog non actif

Ces éléments ne sont pas perdus ; ils ne sont pas des prérequis des lots précédents.

| Élément | Déclencheur |
|---|---|
| Console locale | Les pilotes perdent du temps à lire l’état de leurs environnements. Réutiliser les lectures existantes ; Go et assets statiques, lecture seule, loopback protégé, aucune autre base d’état. |
| Provider européen supplémentaire | Une agence apporte une cible, un cas d’usage et des essais ; pas de développement simultané de trois clouds. |
| Autres registries/CI | Demande réelle sur leur authentification et leur intégration ; conserver les références OCI et contrats génériques dès maintenant. |
| Broker alternatif, Aurora, autre runtime | Une boutique identifiée ne peut pas fonctionner correctement sur le parcours actuel. |
| Observabilité/edge/email multiples | Intégrer d’abord destinations et services existants ; provisionner un fournisseur supplémentaire seulement quand le besoin est testé. |
| SDK indépendant et releases providers autonomes | Un contributeur ou cycle de release est réellement bloqué par la coordination actuelle. |
| MCP | Un usage agent ne peut pas être raisonnablement servi par la CLI structurée et les skills. |
| HA/DR, multi-tenancy de previews | Besoin opérationnel explicite et financement ; pas une case à fermer pour paraître crédible. |

Pas de nouveaux services hébergés, télémétrie obligatoire, marketplace, moteur de workflow, compte MageLift obligatoire ou mécanisme universel de tarification.

## 8. Critères de simplicité observables

Avant d’annoncer la simplicité, vérifier les situations suivantes. Il s’agit de scénarios d’acceptation, pas d’un nouveau tableau de KPI à développer.

1. Un utilisateur installe l’outil et lit l’état sans cloner MageLift, compiler Go ou installer Docker sur un poste qui ne construit rien.
2. Un développeur Magento initialise un dépôt existant sans choisir les détails de réseau, de cluster ou de chiffrement. Un défaut reste inspectable et modifiable.
3. La préparation du compte ne nécessite pas une séance d’accompagnement oral du mainteneur ; les rares actions administrateur/DNS sont identifiées et vérifiées.
4. L’utilisateur ajoute ses propres tests à la CI sans que MageLift les écrase ou prétende que son workflow est invalide uniquement parce qu’il diffère du template.
5. Une panne de credentials ou de migration conduit à une prochaine action précise, sans conseiller automatiquement un destroy/recreate.
6. Une deuxième personne reprend le projet depuis Git et les comptes autorisés, sans récupérer les fichiers secrets du laptop du premier développeur.
7. Le mainteneur corrige un bug GCP sans retester tous les clouds, puis publie une version sans assembler manuellement plusieurs catalogues.
8. Une mise à jour de la CLI ne modifie pas implicitement l’infrastructure ni les données des utilisateurs.

Si un scénario échoue, corriger cette friction avant d’ajouter une nouvelle option d’infrastructure. Une simple réduction du nombre de lignes YAML ne prouve pas la simplicité.

## 9. Instructions de travail pour Codex

Travailler sur un lot actif, en petites modifications couvrant un résultat observable. Avant de créer, rechercher l’équivalent dans le code. Pas de nouvelle phase d’audit général, d’ADR pour chaque fonction ou d’approbation répétée du même plan produit. Actualiser seulement les documents que la modification rend faux.

Garder les protections de secrets, de permissions, d’intégrité et de suppression. Les opérations cloud exigent une cible d’acceptance reconnue et autorisée ; ce plan n’autorise aucune destruction de ressource existante non identifiée. Tags, publication, réécriture d’historique, dépassement d’enveloppe et destruction protégée restent soumis à approbation.

Rapport de fin : **livré ; tests exécutés ; NOT_RUN ; limites ; ressources restantes ; prochain travail utile**. Ne pas transformer un test rouge en mock vert, supprimer une assertion gênante ou déclarer une tâche terminée parce que la checklist a été éditée.

### Prompt de démarrage

```text
Applique CODEX_IMPLEMENTATION_PLAN.md version 1.2. Il remplace les anciens
plans ; n’additionne pas leurs exigences.

Reprends la branche existante depuis wip/all-local-work, sans réécriture
ni écrasement du travail local. Commence par S0, puis S1-S3 pour l’alpha.
Vérifie ce qui est déjà implémenté ; ne le refais pas.

Priorités : recettes construites dans la CI utilisateur, registre choisi
par l’utilisateur, onboarding cloud guidé, workflow non écrasé, erreurs
exploitables, JSON/skills, une release simple. Pas d’images Docker
MageLift centrales, pas de nouvelle plateforme de plugins ni de certification.

Garde GCP comme recette principale et AWS disponible dans son périmètre
expérimental. L’extraction AWS, la console et les autres providers ne
conditionnent pas la première publication.

Conserve les contrats, signatures et protections. Avance sur les tâches
locales autorisées ; pour les essais cloud, exige la cible d’acceptance
et l’enveloppe autorisées. Toute vérification non faite est NOT_RUN.
Pas de tag/publication ni destruction protégée sans approbation.

Pour chaque lot : code ciblé, tests de régression exécutés, documentation
utile, rapport bref. Le résultat doit réduire une manipulation utilisateur
ou une difficulté concrète de maintenance, pas seulement ajouter une abstraction.
```

## 10. Points de référence de la relecture

Le plan v1.1 a été relu intégralement. Les observations de dépôt suivantes ont été revérifiées sur `wip/all-local-work` pendant la révision :

- `docs/onboarding.md` : préparation actuelle du compte, rôles/API à accorder, credentials ADC, domaine/DNS, images de base à construire et contraintes Git décrites dans la documentation. Une affirmation documentaire n’est pas une exécution du code.
- `internal/cli/bootstrap.go` : réutiliser la préparation existante, les paramètres GitHub et les sorties de bootstrap ; compléter leur UX et leur cohérence avec la CI.
- `internal/cli/ci.go` : validation actuelle par comparaison exacte au workflow rendu ; remplacement du fichier différent dans `writeWorkflow`. Rendre les personnalisations utilisateur explicites et protégées.

Les références précédemment examinées restent utiles pour localiser les modifications, sans dispenser de vérifier leur état : `internal/cli/build.go`, `internal/providerhost/`, `providers/gcp/`, `internal/registry/`, `build/src/Magento/`, `sdk/artifact.go`, `intent/ROADMAP.md` et les scripts de développement.

Documentation primaire consultée le 18 octobre 2026 ; relire les versions applicables lors de l’implémentation :

- Google Cloud, fédération d’identité pour la CI : `https://docs.cloud.google.com/iam/docs/workload-identity-federation-with-deployment-pipelines`: permissions, conditions de confiance et identités courtes.
- Google Cloud, GKE et Artifact Registry : `https://docs.cloud.google.com/artifact-registry/docs/integrate-gke`: permissions de pull distinctes des credentials du build.
- Pulumi Automation API : `https://www.pulumi.com/docs/iac/concepts/automation-api/`: orchestration et outillage sous-jacent.
- Docker, cache registry : `https://docs.docker.com/build/cache/backends/registry/` ; secrets : `https://docs.docker.com/build/building/secrets/`.
- GoReleaser, Homebrew Casks : `https://goreleaser.com/customization/publish/homebrew_casks/` ; Scoop : `https://goreleaser.com/customization/publish/scoop/`: publication des métadonnées depuis la chaîne de release.

**Décision finale : moins de choix initiaux et de chantiers transverses ; davantage d’automatisation sur le parcours concret de l’agence. Ne pas transférer la complexité du mainteneur aux utilisateurs pour pouvoir appeler le produit simple.**
