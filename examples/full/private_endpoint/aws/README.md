## AWS Private Link example

Tested with HashiCorp/AWS v5.63.0 Terraform provider.

The Terraform code deploys following resources:
- 1 AWS PrivateLink endpoint with security groups: pl_vpc_foo
- 1 ClickHouse service: red

The ClickHouse service is available from `pl_vpc_foo` PrivateLink connection only, access from the internet is blocked.

## How to run

- Create a VPC into AWS
- Create 2 subnets within the VPC in 2 different AZs.
- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Needed AWS permissions

To run this example, the AWS user you provide credentials for needs the following permissions:

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:CreateTags",
                "ec2:CreateVpc",
                "ec2:DescribeVpcs",
                "ec2:ModifyVpcAttribute",
                "ec2:DescribeVpcAttribute",
                "ec2:CreateSubnet",
                "ec2:DescribeSubnets",
                "ec2:CreateSecurityGroup",
                "ec2:DescribeSecurityGroups",
                "ec2:RevokeSecurityGroupEgress",
                "ec2:AuthorizeSecurityGroupIngress",
                "ec2:CreateVpcEndpoint",
                "route53:AssociateVPCWithHostedZone",
                "ec2:DescribeVpcEndpoints",
                "ec2:DescribePrefixLists",
                "ec2:DescribeNetworkInterfaces",
                "ec2:DescribeSecurityGroupRules",
                "ec2:DeleteVpcEndpoints",
                "ec2:RevokeSecurityGroupIngress",
                "ec2:DeleteSubnet",
                "ec2:DeleteVpc",
                "ec2:DeleteSecurityGroup",
                "ec2:DescribeAvailabilityZones"
            ],
            "Resource": "*"
        }
    ]
}
```
