package model

func (c Config) FindProject(idOrName string) (*Project, bool) {
	for i := range c.Projects {
		if c.Projects[i].ID == idOrName || c.Projects[i].Name == idOrName {
			return &c.Projects[i], true
		}
	}
	return nil, false
}

func (c Config) FindService(idOrName string) (*Project, *Service, bool) {
	for pi := range c.Projects {
		for si := range c.Projects[pi].Services {
			s := &c.Projects[pi].Services[si]
			if s.ID == idOrName || s.Name == idOrName || c.Projects[pi].Name+"/"+s.Name == idOrName {
				return &c.Projects[pi], s, true
			}
		}
	}
	return nil, nil, false
}

func (c Config) AllServices() map[string]Service {
	result := make(map[string]Service)
	for _, project := range c.Projects {
		for _, service := range project.Services {
			result[service.ID] = service
		}
	}
	return result
}
