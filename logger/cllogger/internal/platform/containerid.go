// Package platform regroupe les détails dépendants de l'environnement d'exécution.
package platform

import (
	"os"
	"strings"
)

// ContainerID renvoie un identifiant unique de l'instance en cours d'exécution.
func ContainerID() string {
	// Sur AWS ECS Fargate, l'URI de métadonnées contient le Task ID à la fin.
	if metaURI := os.Getenv("ECS_CONTAINER_METADATA_URI_V4"); metaURI != "" {
		parts := strings.Split(metaURI, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Sinon (Docker classique, Kubernetes, EC2), le hostname est l'ID du conteneur.
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}
