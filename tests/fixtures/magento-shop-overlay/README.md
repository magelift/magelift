# Synthetic shop overlay

Original clean-room test code for the community alpha. This directory is an
overlay for a licensed or Open Source Magento checkout, not a complete shop.
It contains no vendor code, credentials, customer data or database dump.

Copy `app/` into an owned synthetic checkout. Preserve its existing
`app/etc/config.php`, adding `MageLift_AlphaDisabled => 0` to the modules map.
Declare `MageLift/alpha` and locale `en_US` in the existing build static-content
configuration, then commit the fixture before building. The theme extends Luma
and emits `.magelift-alpha-fixture` in its CSS. Verify that the module remains
disabled, the custom theme is deployed and source configuration is unchanged.

Use an isolated clone when changing a patch or theme. Do not apply this overlay
to a customer's shop or treat it as evidence of third-party extension support.
