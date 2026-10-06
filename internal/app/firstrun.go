package app

// MarkNetworkChecked records that the first-run tune ran (on the first
// connect after installing), so it does not run again.
func (s *Service) MarkNetworkChecked() error {
	st := s.x.Settings.Get()
	st.Simple.Checked = true
	return s.x.Settings.Save(st)
}
