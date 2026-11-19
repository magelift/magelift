# Historical acceptance sessions

Earlier MageLift releases grouped several checks into one deployment to limit cloud spend. The run records name the Magento version, services, region and cleanup outcome. Their results apply to those configurations on those dates. [Run records](evidence/README.md)

Contributors now run `make local-gates` for Pulumi mocks and Floci on AWS, GCP and Azure. Run `make ci-act-project` to exercise a generated shop workflow with a local registry. Each shop can use its own staging environment to test behavior that depends on the cloud, such as managed TLS issuance or a database restore. [Provider coverage](capability-matrix.md)
