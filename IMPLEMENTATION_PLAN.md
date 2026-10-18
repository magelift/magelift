# MageLift. Reprise Cursor: uniquement le reste à faire

**Version 1.3, 18 octobre 2026.** Remplace le plan v1.2 comme file de travail active ; ne pas additionner les anciennes checklists.  
**Exécution :** Cursor, avec Grok 4.7 et/ou Composer 2.5 au choix du mainteneur. Aucune dépendance à Codex.  
**Objectif immédiat :** terminer l’acceptance Magento 2.4.9 sur les ressources GCP déjà présentes, intégrer les dernières corrections au logiciel distribué, nettoyer la campagne, puis publier une alpha communautaire bornée.

## 1. Point de reprise et règles non négociables

### Références examinées

| Dépôt | Branche | HEAD lu pour cette révision |
|---|---|---|
| `magelift/magelift` | `reboot/community-alpha` | `1d1d1f23890c0d4843e0b368e7ade066fcef475f` |
| `magelift/acceptance-magento-249` | `main` | `e4e133ee085a8de780d90fac937d11525b80fadf` |

L’historique comprend 79 commits MageLift depuis `3ac1b844a7117752f3a5045054587508c16dcf6c`, et 11 commits de la fixture. Le code, les rapports et les derniers jobs CI ont été lus pour ce plan ; aucun test ou inventaire GCP n’a été exécuté par son auteur. Les modifications locales de la session Codex ne sont pas visibles sur GitHub. **Relever le vrai HEAD et le travail non committé avant toute action ; ces SHA ne sont pas des instructions de reset.** [S1-S4]

### État transmis par le mainteneur, à préserver

- L’indexation **Catalog Search Full Text est passée** et la recherche dans le navigateur retrouve les produits synthétiques. Récupérer les preuves et leur identité d’image ; ne pas recommencer le chantier recherche.
- La validation finale du navigateur reste à refaire **avec les assets corrigés**. La présence de cette correction dans une image ou dans les commits doit être établie, pas supposée.
- La sonde cron n’a pas été correctement exécutée : elle a reçu un horodatage au mauvais format. Reprendre avec le format **ISO 8601 exigé par la sonde** ; cela n’établit pas une panne du cron.
- Les ressources de test sont encore présentes sur GCP. Elles constituent **la campagne à reprendre**, pas un environnement à remplacer.

Le dernier workflow distant lu, `39135412332`, tentative 2, termine le 18 octobre à 14:37 UTC : validation et build/signature réussis, déploiement en échec. Ses logs indiquent des migrations terminées et un service stabilisé, puis l’échec du Job de readiness. Rapprocher ce résultat des dernières observations locales rapportées ; il ne suffit ni à nier celles-ci, ni à déclarer la CI réparée. [S5]

### Politique de campagne GCP

**Conserver la même infrastructure pendant les tests ; la supprimer après le dernier essai nécessaire et l’export des preuves.** Une fin de commande, de session Cursor ou de quota modèle n’est pas une fin de campagne.

Réutiliser projet, région, noms de stack, base, réseau, cluster, volumes, secrets, clé de chiffrement et backend. Le journal et la dernière CI désignent `digital-lab-341608`, `us-central1`, le projet MageLift `magelift-org249-ec2a332`, l’environnement `test` et la stack `magelift-org249-ec2a332-test-gcp-gke-autopilot`. Ce sont des **pistes de rapprochement**, pas une autorisation de supprimer par préfixe. Les identifiants précis se récupèrent dans la configuration d’acceptance ignorée, les reçus, l’état et les API natives. Ne pas basculer vers un autre projet anciennement autorisé. [S3, S5]

Ne pas relancer par défaut un wrapper qui impose un préfixe neuf, refuse la reprise ou détruit sur `EXIT`. Les anciennes contraintes « fresh stack » et « destroy after every run » sont remplacées, pour cette campagne explicitement retenue, par une reprise contrôlée et un teardown final vérifié. Réutiliser les helpers existants ; ne pas développer une nouvelle plateforme de campagnes.

**Un seul processus/agent responsable des mutations cloud à la fois.** Un second modèle peut lire le code ou revoir un patch, mais ne déploie, ne réindexe et ne nettoie pas en parallèle. Pas de suppression de verrou sans vérifier son propriétaire et l’absence d’opération active.

### Architecture conservée

Go/Cobra, Pulumi, YAML, PHP, manifeste existant, provider GCP signé et recettes embarquées restent en place. Les images se construisent dans la CI utilisateur et sont publiées dans son registre. Aucun retour aux images Docker centrales MageLift, à une extraction AWS obligatoire, à une migration de broker ou à une certification exhaustive.

Les anciennes implémentations S0/S1/S2 ne sont plus des lots à refaire. Une vérification finale peut révéler une régression ciblée ; elle n’autorise pas à reconstruire les sous-systèmes déjà livrés. L’historique détaillé figure dans `MageLift_Releve_Avancement_v1.3.md`, relevé séparé non nécessaire à chaque session.

## 2. File de travail active

| Lot | Résultat | Dépendance |
|---|---|---|
| R0 | Reprise locale et inventaire de la campagne, sans changement d’infrastructure | Immédiat |
| R1 | Assets/navigateur, cron et cause de la readiness résolus | R0 |
| R2 | Corrections dans un artefact distribué ; CI et seconde release validées sur la même stack | R1 ; préparation possible pendant R1 |
| R3 | Preuves exportées et ressources temporaires nettoyées | Dernier test GCP de R2 terminé |
| R4 | Documentation, distribution publique et statut alpha cohérents | R2 ; publication après R3 |

R4 peut être préparé en parallèle des attentes cloud, sans modifier l’infrastructure. **Ne pas maintenir GCP allumé pour attendre une relecture de prose, un test d’installation macOS, un tag ou l’accord de publication.** Une modification de code runtime découverte avant nettoyage entraîne uniquement les tests concernés, sur la stack conservée.

Une preuve existante valable ferme l’item correspondant. Ne rejouer que ce qui est absent, devenu invalide par une correction, ou nécessaire au contrôle final de l’artefact réellement distribué.

## R0: Reprendre la session, pas le projet

**Zones :** les deux checkouts locaux, `AGENTS.md`, `intent/ROADMAP.md`, début et dernières sections de `intent/community-alpha/verification.md`, fichiers de travail ignorés `.magelift/`, configuration d’acceptance et CI. [S2-S4]

1. Relever `git status`, branches, HEAD, commits locaux non poussés et changements depuis les SHA ci-dessus, dans les deux dépôts. Ne pas lancer `reset --hard`, `clean -fd`, checkout forcé, rebase ou restauration automatique. Préserver les changements des assets, scripts de sonde, données synthétiques et preuves de navigateur.
2. Retrouver les commandes réellement utilisées par Codex et le dernier compte rendu local. Chercher dans les dossiers ciblés ; ne pas parcourir tout Magento/vendor pour chaque diagnostic. Distinguer fichiers modifiés sur disque, corrections injectées dans un conteneur et corrections déjà incluses dans une image signée.
3. Établir en lecture seule l’identité de l’environnement : projet/région/stack, cluster, namespace, workload web/cron, SQL, cache, recherche/PVC, buckets, registre, URL, état distant et référence/version de clé. Consigner les objets de bootstrap séparément. Vérifier les opérations en cours, les Jobs non terminés et les verrous avant toute relance.
4. Relever versions et digests de la CLI, du provider, de l’image réellement exécutée et du manifeste ; distinguer digest d’index OCI et manifeste de plateforme. Conserver le lien vers le build CI producteur. Ne pas déduire l’image courante du seul dernier tag Git.
5. Mettre à jour le début du rapport existant avec un **point de reprise court** : prochaine action, preuves déjà retrouvées, blocages, identités non secrètes et emplacement privé des reçus. Garder l’historique en dessous sans le réécrire. Les secrets, états bruts, logs sensibles et tokens restent ignorés et protégés.
6. Installer ce plan sous un nom canonique, par exemple `IMPLEMENTATION_PLAN.md`, en remplaçant le plan actif précédent ; ajuster uniquement les références actives dans `AGENTS.md`/roadmap. Ne pas laisser deux plans concurrents. Enregistrer la nouvelle politique de conservation GCP. Les anciennes exigences de Codex, d’un outil de reformulation ou d’un MCP indisponible ne bloquent pas le code ni la documentation.

**Sortie :** les deux workspaces et la stack sont identifiés, les corrections locales sont préservées, aucune ressource n’est recréée. L’observation recherche est classée « preuve retrouvée » ou « réussite rapportée, à raccorder à un digest », jamais « implémentation à recommencer ».

## R1: Fermer les derniers contrôles applicatifs

### R1a. Assets et navigateur

**Zones :** correction locale retrouvée ; sinon `images/`, `build/src/Magento/`, `internal/build/`, bindings/runtime GCP et configuration de la fixture, selon la cause réelle.

- Déterminer ce qui a été corrigé : fichiers générés, thème/locale, URL/version statique, permissions, chemin nginx, cache ou configuration runtime. Aucun diagnostic n’est imposé à l’avance.
- Vérifier le frontend en HTTPS, son CSS, les scripts nécessaires, un produit synthétique et la recherche déjà observée. Contrôler les requêtes réseau utiles, types de contenu, absence de 404/500 bloquants, de mixed content et d’erreurs JavaScript empêchant l’usage. Une page HTML 200 sans ses assets ne ferme pas l’item.
- Distinguer cache navigateur, cache applicatif et absence d’un fichier dans l’image. Un contournement local ou une copie manuelle de CSS dans un Pod est un diagnostic, pas la correction distribuable.
- Porter la correction dans la recette/configuration appropriée, avec le test ciblé qui la couvre. Si elle est déjà committée et distribuée, ne pas la réécrire. Ne pas vider la base, changer de thème arbitrairement, ouvrir les médias privés ou activer le mode développeur pour obtenir un écran acceptable.
- Conserver quelques captures et assertions sur des contenus synthétiques, URL expurgée si nécessaire, date et digest testé. Aucun nouveau framework de tests navigateur si les outils existants suffisent.

### R1b. Cron : corriger l’appel avant de modifier l’ordonnanceur

- Retrouver la sonde et lire son contrat d’entrée. Générer **une borne temporelle UTC ISO 8601** avant la fenêtre observée, par exemple avec Python `datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')`. Ne pas réutiliser un timestamp d’exemple, une date SQL ou un epoch lorsque le parseur attend autre chose.
- Réexécuter la sonde avec ses vrais arguments. Une erreur de parsing reste « contrôle non exécuté », pas « cron en panne » ni PASS.
- Prouver qu’une tâche cron pertinente s’exécute avec succès après la borne choisie, via le scheduler réellement déployé et les données/logs appropriés. Séparer exécution naturelle et appel manuel `cron:run` de diagnostic. Une ligne ancienne dans `cron_schedule` ou un simple processus vivant ne suffit pas.
- Utiliser une attente bornée cohérente avec le planning existant. En cas d’échec réel, examiner commande du workload, planning, timezone, permissions et erreurs Magento avant de patcher. Pas d’attente infinie ni de nouveau service cron.

### R1c. Readiness et résultat du déploiement CI

**Point de départ :** run `39135412332`, job `117701195464`, tentative 2. Les logs lus contiennent successivement `migrations complete`, `service stabilized`, puis :

```text
provider deploy-app-phase: upstream: magento readiness probe:
wait for migrate Job: job failed: Job has reached the specified backoff limit
```

- Rapprocher cet échec des corrections locales ultérieures. Lire les logs du **Job de probe exact** et son code de sortie avant qu’ils disparaissent. Le libellé interne « migrate Job » ne prouve pas, à lui seul, qu’une migration a été relancée.
- Distinguer un défaut réel de l’application d’une sonde mal configurée ou d’un ordre de validation incorrect. Corriger la cause avec une régression ; ne pas remplacer la probe par `/health`, ignorer son retour ou accroître aveuglément les retries.
- Conserver les corrections déjà livrées de bootstrap de base vide, propagation RPC des échecs et absence de retries automatiques des migrations. Ne pas recréer la base pour revenir à l’erreur initiale.
- Le succès navigateur rapporté ne clôture pas automatiquement ce défaut : **la commande de déploiement doit finir avec le bon résultat sur le parcours utilisateur**. La validation finale passe par R2, après intégration des corrections.

**Sortie R1 :** assets et cron ont un résultat explicite ; la cause de readiness est corrigée ou précisément bloquée. Les corrections nécessaires sont localisées et testables. Aucun changement de topologie n’a été introduit pour résoudre un défaut d’assets ou de sonde.

## R2: Valider le chemin distribué et la seconde release

**Zones :** corrections R1, pipeline/release existants, `.github/workflows/magelift.yml` et `magelift.providers.lock` de la fixture, contrats de déploiement, observations et sondes existantes.

### Intégrer une fois, puis vérifier sur l’environnement conservé

1. Mettre les corrections pérennes dans le bon dépôt. Les fixtures, contenus synthétiques et personnalisation d’acceptance restent dans leur périmètre ; un correctif générique de runtime reste dans MageLift. Aucun code d’un employeur ni secret ne rejoint le dépôt public.
2. Relancer les seuls tests affectés, puis une fois `make verify-community` sur le candidat consolidé. Employer `make workflow-check` et les générateurs existants lorsque concernés. S0 et les sous-systèmes de S1/S2 ne sont pas à réimplémenter. Pas de `make verify`/race global ou matrice tous clouds comme détour obligatoire.
3. Réutiliser le canary `v0.0.0-canary.1d1d1f23890c0d4843e0b368e7ade066fcef475f` si le logiciel est inchangé. Sinon produire le candidat correspondant au vrai commit via la chaîne existante, avec l’approbation requise pour le tag/publication. Ne pas publier un canary à chaque sonde, modifier un tag déjà signé ni désactiver la vérification pour exécuter du code local.
4. Dans la fixture, aligner version CLI, lock provider et workflow sur ce candidat ; conserver les modifications légitimes du workflow. `MAGELIFT_ACCEPTANCE_CONFIG` reste matérialisée dans `magelift.acceptance.yaml` ignoré. Ne pas réécrire le YAML suivi en CI ni publier les credentials fédérés.
5. Utiliser un runner propre pour les parties qu’une correction impose de rejouer, **pas une infrastructure GCP neuve**. La CI construit l’image du projet et la pousse dans son registre. Un changement de CLI/provider seul peut redéployer le digest compatible existant ; un changement de recette applicative nécessite une nouvelle image. Les assets finals doivent provenir du build, pas d’une intervention manuelle après déploiement.
6. Avant apply, examiner le plan : aucune recréation imprévue de GKE, SQL, réseau ou données. Le correctif `014dd2d` évite justement la gestion de champs Autopilot calculés qui avait proposé des remplacements. Ne pas inverser ce choix ni changer de tier/région pour une préférence esthétique. En cas de remplacement inattendu, arrêter cette mutation et diagnostiquer.
7. Obtenir un résultat CI concluant jusqu’à la validation applicative. Conserver SHA, digests, jobs et sorties nécessaires ; un build PASS suivi d’un deploy FAIL reste un pipeline incomplet.

### Compléter uniquement les preuves absentes

| Contrôle | Ce qui reste à prouver, sauf preuve valide déjà retrouvée |
|---|---|
| Frontend et recherche | Refaire le contrôle final des assets sur l’image distribuée ; rattacher indexation et recherche synthétique au digest. Ne réindexer qu’en cas de nécessité réelle. |
| Cron | Résultat de R1b sur le workload final, après une borne temporelle valide. |
| Queue DB | Publication et traitement d’un message synthétique par le chemin applicatif réellement annoncé. Ne pas ajouter RabbitMQ/Artemis pour fermer cette recette. Une table présente ne prouve pas un message consommé. |
| Médias | Upload/lecture d’un média de test et refus d’un chemin privé/sensible avec le bucket non public. Conserver les règles d’accès existantes. |
| Seconde release | Une modification applicative synthétique observable apparaît après déploiement ; produits/contenu/média créés avant restent lisibles avec les mêmes identifiants utiles. Une marque SQL PDO seule ne remplace pas ce contrôle applicatif. |
| Promotion et accès au registre | Le déploiement d’un artefact existant ne rappelle pas le builder. Le runtime peut récupérer son image avec son identité de pull, sans emprunter le jeton éphémère du build. Réutiliser le rollover des workloads comme preuve quand possible ; ne pas renouveler le cluster pour ce test. |
| Exploitation et incident | `outputs`, état/health, logs et exec via la CLI distribuée, sans injecter artificiellement `imageDigest` dans le YAML pour une lecture ; JSON/non-interactif, code d’échec utile et export de métadonnées sans secret. Réutiliser l’incident readiness comme preuve de diagnostic s’il couvre le contrat, plutôt que provoquer une panne destructive. |
| Préparation réexécutable | Retrouver la preuve du bootstrap relançable. Si elle manque encore, exécuter sa voie de revue/relance avec les mêmes identités : pas de renouvellement de clé, d’élargissement de droits ou de nouvelle adoption. |

Ne pas ouvrir une deuxième stack complète pour prouver une promotion. L’alpha doit distinguer la réutilisation effective du digest sur le chemin testé d’une promotion inter-comptes/inter-registres non encore essayée. Une restauration universelle, tous les OS et un pilote humain ne bloquent pas cette campagne.

Relever avant teardown la configuration de runner réellement utilisée, les durées déjà présentes dans les jobs et le coût brut disponible de la campagne. Le net après crédits n’est pas le prix du produit. Si le coût final est retardé ou la mémoire de pic non mesurée, écrire cette limite ; ne pas ajouter un banc FinOps ou une nouvelle campagne uniquement pour la mesurer.

**Sortie R2 :** le parcours GCP annoncé et sa seconde release sont prouvés avec les corrections distribuées ; les limites restantes sont nommées. Les résultats sont exportés hors du stockage qui va être détruit. **Passer à R3 sans attendre la publication publique.**

## R3: Nettoyer la campagne temporaire GCP

**But :** aucun runtime de test laissé inutilement actif ; aucun objet étranger supprimé. Ce teardown est à la fois la fermeture de campagne et le test du chemin destroy.

Le mainteneur demande expressément le nettoyage de ces ressources de test à la fin. Cette instruction couvre les objets temporaires de la campagne **identifiés et possédés** ; elle ne couvre pas toute la facturation/projet GCP, les anciennes stacks étrangères, les DNS de l’organisation ou les ressources protégées dont le rattachement n’est pas prouvé.

### Avant la première suppression

- Exporter le résumé des tests, les références de commits/images, les preuves browser/cron/CI, l’inventaire natif et les reçus. Conserver hors Git public l’état chiffré et les références utiles à la récupération. Tester la lisibilité de cette sauvegarde par l’opérateur autorisé ; aucune clé en clair dans le rapport.
- Vérifier qu’aucun déploiement ou test requis n’est encore actif. Neutraliser **les déclencheurs de déploiement de cette fixture uniquement** pour éviter qu’un push de README après nettoyage recrée la stack. Préserver la validation hors cloud et documenter le mécanisme de réactivation ; ne pas désactiver toute la CI du projet MageLift.
- Dresser la liste précise : ressources de stack ; objets créés par les contrôleurs Kubernetes ; ressources temporaires de bootstrap ; artefacts/buckets de la campagne ; objets externes à préserver. Utiliser état, labels, reçus et références croisées, jamais le seul préfixe.
- Utiliser une version contenant `1d1d1f2` ou son successeur. Le destroy doit préparer un `PreviewDestroy` depuis l’état, sans exécuter le programme applicatif, sans reconstruire d’image et sans relancer Magento. Lire le plan de suppression ; ne pas appeler un déploiement pour rendre un destroy possible. [S6]

### Ordre de nettoyage

| Ordre | Opération |
|---|---|
| 1 | Détruire la stack avec la voie MageLift/Pulumi existante ; laisser les contrôleurs supprimer leurs objets dépendants lorsque possible. Ne pas effacer d’abord le cluster ou forcer ses finalizers. |
| 2 | Vérifier par API native les ressources de la campagne, notamment GKE/nœuds, Jobs, disques/PVC de recherche, objets LB/NEGs/backends/health checks/certificats/adresses, SQL et sauvegardes, Valkey, réseau/peering/plages réservées. Nettoyer seulement les résidus identifiés ; lire leurs dépendances avant suppression. |
| 3 | Après disparition des consommateurs, nettoyer médias, images et caches de test, objets versionnés/backups temporaires selon leur politique effective. Préserver ce qui est partagé ou nécessaire à une release publique ; un registre utilisateur entier n’est pas supprimé parce qu’il contient une image de test. Distinguer rétention imposée, soft delete et objet encore actif. |
| 4 | Retirer seulement les entrées DNS de test prouvées et les identités/HMAC/secrets/fédération/grants de bootstrap propres à cette campagne qui ne servent plus ailleurs. Préserver l’accès de l’opérateur de nettoyage jusqu’au bout. Les grants et variables GitHub sont retirés à partir de leurs reçus, pas avec une réécriture globale des politiques IAM. |
| 5 | Après confirmation de disparition des objets à gérer, supprimer le backend d’état temporaire et sa clé dédiée si leur propriété et absence de partage sont démontrées. Si des ressources restent, **garder état et clé** pour finir leur récupération. |

Utiliser les outils de nettoyage et inventaires déjà présents. Une suppression native ciblée est acceptable pour un objet orphelin prouvé, après diagnostic et traçabilité ; elle ne doit pas masquer une régression de MageLift. Ajouter le correctif de code nécessaire s’il est petit et lié au parcours, sans ouvrir un framework de garbage collection.

### Résultat final

La sortie doit distinguer **absence confirmée**, **suppression en cours**, **rétention obligatoire avec échéance**, **objet partagé préservé** et **vérification impossible**. Une erreur d’autorisation n’est pas une liste vide ; un exit 0 Pulumi n’est pas un inventaire natif.

Vérifier notamment les objets créés hors checkpoint après un échec, les disques conservés et les objets LB gérés par contrôleur. Ne pas supprimer les dépôts Git, les releases publiques ou la zone DNS parente. Refaire une lecture native après stabilisation des suppressions, sans créer de ressources.

**Sortie R3 :** aucun runtime temporaire de la campagne encore actif ; inventaire final et restes éventuels explicités. `CLEANUP_PASS` ne peut pas couvrir un objet payant oublié ou un contrôle non exécuté. Une rétention contrainte peut être notée `PENDING` avec propriétaire, coût/incertitude et action/échéance précise ; ne pas annoncer « tout supprimé ».

Si la session s’interrompt avant la fin des tests, enregistrer la rétention explicite et le prochain contrôle, sans destruction précipitée. Si les tests sont durablement bloqués, ne pas garder indéfiniment la campagne allumée : fournir au mainteneur un état expurgé et une décision ciblée de conservation ou nettoyage. Ne jamais demander à recréer tout le projet par défaut.

## R4: Fermer la distribution et les documents, sans nouvelle campagne

**Zones :** README, `docs/onboarding.md`, `docs/release-readiness.md`, installation, `SUPPORT.md`, skills concernés, rapport courant, README de la fixture, workflows de release et publication du site.

1. Mettre les statuts actuels en accord avec R1-R3. Le README d’acceptance et plusieurs paragraphes du rapport sont plus anciens que les derniers runs : corriger le résumé actuel et garder les anciens essais comme historique, pas les convertir rétroactivement en succès. Ne pas confondre fixture GitHub source avec toutes les installations Composer/Adobe Commerce.
2. Décrire une seule recette Magento Open Source 2.4.9, avec les patches de sécurité déjà identifiés dans `ACCEPTANCE_SECURITY_PATCHES.md`, le tuple réellement utilisé, ses limites et son environnement de test. Ne pas appliquer une nouvelle vague de mises à jour pour embellir le plan ; une vulnérabilité bloquante nouvelle se traite explicitement. [S7]
3. Vérifier l’onboarding contre les commandes distribuées : starter simple, bootstrap, CI utilisateur non écrasée, image et registre utilisateur, sortie de déploiement, diagnostic et teardown. Pas de retour aux étapes manuelles de fabrication des bases MageLift ni à un accès obligatoire au workspace du mainteneur.
4. Préparer l’intégration depuis `reboot/community-alpha` vers la branche/canal réellement publié. `main` ancien ne doit pas continuer à fournir un installateur/site et un guide incompatibles avec le logiciel recommandé. La preuve d’un installateur extrait d’un tag ne vaut pas preuve de l’URL publique habituelle. Faire les smoke tests de distribution depuis le canal annoncé, sans reprovisionner Magento.
5. Réutiliser les workflows signés existants et la version suivante réellement disponible. Garder les tests de confiance et les métadonnées Brew/Scoop ; qualifier seulement les canaux annoncés. Une plateforme non essayée peut être documentée comme telle sans bloquer Linux/GCP. Pas de nouvelle chaîne de release par modèle, provider ou package manager.
6. Conserver la prise en compte rapide des issues, une procédure de reproduction/diagnostic et des PRs accessibles. Pas d’astreinte, SLA chiffré, couverture multi-cloud ou capacité de restauration ajoutés à la promesse. Un essai par une personne extérieure reste utile ; s’il manque, ne pas déclarer l’ergonomie validée, sans immobiliser GCP en l’attendant.
7. Obtenir l’approbation explicite requise pour merge protégé, tag et publication. Si elle manque, livrer la PR/le candidat et les commandes exactes à approuver ; les ressources temporaires déjà inutiles doivent être nettoyées quand même.

**Sortie R4 :** alpha communautaire publiée, ou candidat prêt explicitement en attente d’approbation ; documentation cohérente ; aucun test GCP conservé pour faire vitrine. Une éventuelle documentation sur les identifiants cloud reste expurgée.

## 3. Ce qui reste après l’alpha, mais ne doit pas retarder R0-R4

| Travail restant | Déclenchement et limite |
|---|---|
| Premier pilote agence / ancienne S4 | Import dans un environnement isolé, sauvegarde/restauration des données et médias avec la clé requise, reprise depuis un deuxième opérateur. À valider avant de recommander la migration de données commerciales réelles. Utiliser les primitives existantes. |
| AWS / ancienne S5 | Une recette ECS et un test groupé, après vérification du crédit restant (~150 USD annoncés, ~100 au plus planifiés et ~50 de réserve). Pas de dépense AWS dans la campagne GCP actuelle. Extraction provider seulement sur besoin mesuré. |
| Previews et console | Selon un pilote : rendre fiable expiration/nettoyage et visibilité avec les opérations existantes. Pas de plateforme multi-tenant, nouveau contrôle cloud ou terminal web avant nécessité. |
| Provider européen / autres intégrations | Un cas utilisateur et un contributeur/pilote engagé ; conserver le statut expérimental. Pas de matrice universelle ni gel prématuré du SDK. |

Ces points ne sont pas des tâches automatiques à enchaîner après une limite de contexte. Une fois R3/R4 atteints, restituer le résultat ; ne pas créer une nouvelle campagne de dépenses pour remplir ce tableau.

## 4. Discipline de reprise dans Cursor

Le premier modèle actif pilote le lot et les mutations. L’autre peut implémenter un correctif isolé sur une branche/worktree distinct ou relire un diff ; intégration par le pilote, sans changements concurrents dans les mêmes fichiers. Pas de double revue systématique ni de dépendance obligatoire à une version particulière du modèle.

À chaque session, lire ce plan, le **résumé courant** du rapport et les seules zones du lot. Les skills du dépôt restent du texte utilisable sans Codex. Aucun besoin de redéployer pour retrouver son contexte. Les contraintes de privilèges, de signature, de secrets et de propriété restent impératives.

**Statuts simples, sans nouveau moteur :** `PASS` avec référence de preuve ; `FAIL` avec cause ; `NOT_RUN` avec raison ; `REPORTED_PASS` pour l’observation du mainteneur non encore raccordée à une preuve retrouvée. Conserver séparément résultat applicatif et nettoyage. Ne pas transformer un crash de sonde en PASS, ni invalider une preuve historique seulement parce que le modèle a changé.

Compte rendu court à chaque point d’arrêt :

```text
Lot / prochaine action :
SHA MageLift / SHA fixture / corrections locales restantes :
CLI + provider + image/manifest réellement utilisés :
Contrôles nouvellement PASS / FAIL / NOT_RUN et preuves :
Dernière mutation et opération/verrou encore actifs :
GCP conservé pour quels tests / emplacement privé de l’inventaire :
Nettoyage : NOT_STARTED / IN_PROGRESS / PASS / PENDING / BLOCKED :
Limites ou approbation réellement manquante :
```

Ne pas y joindre de secret, dump, kubeconfig, URL présignée ni journal complet. La copie privée nécessaire à la récupération doit rester accessible à l’opérateur autorisé jusqu’à la fin du nettoyage.

### Prompt de démarrage

```text
Lis IMPLEMENTATION_PLAN.md v1.3 et reprends R0, puis uniquement le reste à faire.
Travaille dans les deux dépôts existants ; ne recrée ni la branche ni GCP.
Préserve les modifications locales de Codex et rapproche-les des HEAD distants.

L’indexation et la recherche synthétique ont déjà réussi selon le mainteneur.
Reste à raccorder leurs preuves, valider le navigateur avec les assets corrigés,
reprendre la sonde cron avec son ISO 8601 requis, et obtenir un déploiement CI
concluant. Le dernier run distant 39135412332 a réussi le build mais échoué
sur la readiness après migrations : ne le confonds pas avec l’état local actuel.

Réutilise la stack active pendant tous les tests. Pas de destroy/recreate par
sonde, session ou modèle. Une seule autorité de mutation. Conserve état et clé.
Porte les corrections dans les recettes/artefacts distribués, valide la seconde
release, exporte les preuves, puis nettoie les ressources temporaires possédées
et vérifie l’inventaire natif. N’attends pas la publication pour nettoyer.

Ne refais pas S0/S1/S2. Aucun nouveau framework, image Docker centrale,
provider, broker ou programme de certification. Pas d’AWS dans cette campagne.
Tags/publication et actions protégées restent soumis à l’accord requis.
Si un accès manque, note le blocage précis et avance sur les tâches indépendantes.
À chaque arrêt, laisse le point de reprise court demandé dans le plan.
```

## 5. Sources pour retrouver les preuves, pas pour rouvrir les anciens lots

- **[S1] Historique MageLift** : https://github.com/magelift/magelift/compare/3ac1b844a7117752f3a5045054587508c16dcf6c...1d1d1f23890c0d4843e0b368e7ade066fcef475f
- **[S2] Roadmap actuelle** : https://github.com/magelift/magelift/blob/1d1d1f23890c0d4843e0b368e7ade066fcef475f/intent/ROADMAP.md
- **[S3] Rapport de vérification** : https://github.com/magelift/magelift/blob/1d1d1f23890c0d4843e0b368e7ade066fcef475f/intent/community-alpha/verification.md: commencer par le résumé courant et la fin, pas par les anciens ML-00.
- **[S4] Fixture et historique** : https://github.com/magelift/acceptance-magento-249/commits/e4e133ee085a8de780d90fac937d11525b80fadf/ ; README-ACCEPTANCE.md et workflow au même SHA.
- **[S5] Dernier run examiné** : https://github.com/magelift/acceptance-magento-249/actions/runs/39135412332: tentative 2 ; job de déploiement 117701195464, job build 117701195605.
- **[S6] Teardown sans exécution du programme** : https://github.com/magelift/magelift/commit/1d1d1f23890c0d4843e0b368e7ade066fcef475f
- **[S7] Patches déjà documentés** : https://github.com/magelift/acceptance-magento-249/blob/e4e133ee085a8de780d90fac937d11525b80fadf/ACCEPTANCE_SECURITY_PATCHES.md

**Fin de mission immédiate : une campagne GCP concluante et nettoyée, des corrections réellement distribuables et un statut public honnête. Pas une nouvelle plateforme à construire.**
