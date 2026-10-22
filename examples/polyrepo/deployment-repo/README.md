# Customer API deployment repository

This directory represents an independently versioned GitOps repository. It is
referenced by `../fleck.yaml` together with the separate application repository.
The Service intentionally targets port `9090`, while the selected container
declares port `8080`. The runtime Secret is intentionally absent and is reported
as unverified rather than definitely missing.
