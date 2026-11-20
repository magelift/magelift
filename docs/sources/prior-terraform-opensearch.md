# Prior Magento and AWS OpenSearch deployment

External reference recorded on 2026-07-22.

An earlier commerce project used Terraform to provision AWS OpenSearch
Service, connect it to Magento and configure VPC networking. The team
tested search capacity in staging and prepared the production
infrastructure. The record omits the private repositories.

The shop used the Smile ElasticSuite client with managed
`OpenSearch_3.x` domains. Magento reached the domain over HTTPS on port 443
inside the VPC. The domain policy and security groups controlled access.
HTTP authentication was off and the setup used no SigV4 proxy.

## How MageLift uses the reference

- Size managed OpenSearch for the JVM load of the shop's catalog. A
  `t3.small.search` can be too small under load.
- Connect the Magento search client over HTTPS inside the VPC.
- Use a static `/health` endpoint for ALB and ECS liveness checks so those
  checks can run without bootstrapping Magento.

MageLift supports the native Magento OpenSearch client and supplies the
endpoint to ElasticSuite. For `searchMode:serverless`, the OpenSearch
Serverless path uses a local signing proxy for requests that require SigV4.
The prior Terraform deployment covered provisioned domains. Use MageLift's
run records and provider tests to assess its own search paths.
[Provider support](../capability-matrix.md)
