// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/gardener/gardener/extensions/pkg/util"
	"github.com/gardener/gardener/extensions/pkg/webhook/cloudprovider"
	gcontext "github.com/gardener/gardener/extensions/pkg/webhook/context"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	apiaws "github.com/gardener/gardener-extension-provider-aws/pkg/apis/aws"
	"github.com/gardener/gardener-extension-provider-aws/pkg/aws"
)

// NewEnsurer creates cloudprovider ensurer.
func NewEnsurer(scheme *runtime.Scheme, logger logr.Logger) cloudprovider.Ensurer {
	return &ensurer{
		logger:  logger,
		decoder: serializer.NewCodecFactory(scheme, serializer.EnableStrict).UniversalDecoder(),
	}
}

type ensurer struct {
	logger  logr.Logger
	decoder runtime.Decoder
}

// EnsureCloudProviderSecret ensures that cloudprovider secret contains
// the shared credentials file.
func (e *ensurer) EnsureCloudProviderSecret(_ context.Context, _ gcontext.GardenContext, new, _ *corev1.Secret) error {
	if new.ObjectMeta.Labels != nil && new.ObjectMeta.Labels[securityv1alpha1constants.LabelWorkloadIdentityProvider] == "aws" {
		if _, ok := new.Data[securityv1alpha1constants.DataKeyConfig]; !ok {
			return errors.New("cloudprovider secret is missing a 'config' data key")
		}
		workloadIdentityConfig := &apiaws.WorkloadIdentityConfig{}
		if err := util.Decode(e.decoder, new.Data[securityv1alpha1constants.DataKeyConfig], workloadIdentityConfig); err != nil {
			return fmt.Errorf("could not decode 'config' as WorkloadIdentityConfig: %w", err)
		}

		new.Data[aws.RoleARN] = []byte(workloadIdentityConfig.RoleARN)
		new.Data[aws.WorkloadIdentityTokenFileKey] = []byte(aws.WorkloadIdentityMountPath + "/token")
		new.Data[aws.SharedCredentialsFile] = []byte("[default]\n" +
			fmt.Sprintf("web_identity_token_file=%s\n", aws.WorkloadIdentityMountPath+"/token") +
			fmt.Sprintf("role_arn=%s", workloadIdentityConfig.RoleARN),
		)
		return nil
	}

	if _, ok := new.Data[aws.AccessKeyID]; !ok {
		return fmt.Errorf("could not mutate cloudprovider secret as %q field is missing", aws.AccessKeyID)
	}
	if _, ok := new.Data[aws.SecretAccessKey]; !ok {
		return fmt.Errorf("could not mutate cloudprovider secret as %q field is missing", aws.SecretAccessKey)
	}

	e.logger.V(5).Info("mutate cloudprovider secret", "namespace", new.Namespace, "name", new.Name)
	new.Data[aws.SharedCredentialsFile] = []byte("[default]\n" +
		fmt.Sprintf("aws_access_key_id=%s\n", string(new.Data[aws.AccessKeyID])) +
		fmt.Sprintf("aws_secret_access_key=%s", string(new.Data[aws.SecretAccessKey])),
	)

	return nil
}
